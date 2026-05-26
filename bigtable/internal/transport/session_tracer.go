// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc/status"
)

var (
	// Metrics registered under bigtable.googleapis.com/internal/client/
	// The formatter in otel_metrics.go will append this prefix and replace . with /
	// So "session.durations" -> "bigtable.googleapis.com/internal/client/session/durations"

	sessionDurations     metric.Float64Histogram
	sessionOpenLatencies metric.Float64Histogram
	sessionUptime        metric.Float64Histogram
)

// InitializeMetrics registers the session metrics.
func InitializeMetrics(meterProvider metric.MeterProvider) error {
	if meterProvider == nil {
		return nil // No-op if not provider available
	}
	meter := meterProvider.Meter("cloud.google.com/go/bigtable/internal/client")

	var err error
	sessionDurations, err = meter.Float64Histogram("session.durations",
		metric.WithDescription("Duration of operations within a session"),
		metric.WithUnit("ms"))
	if err != nil {
		return fmt.Errorf("failed to create session.durations histogram: %w", err)
	}

	sessionOpenLatencies, err = meter.Float64Histogram("session.open_latencies",
		metric.WithDescription("Latency to open a session"),
		metric.WithUnit("ms"))
	if err != nil {
		return fmt.Errorf("failed to create session.open_latencies histogram: %w", err)
	}

	sessionUptime, err = meter.Float64Histogram("session.uptime",
		metric.WithDescription("Total lifetime of a session"),
		metric.WithUnit("ms"))
	if err != nil {
		return fmt.Errorf("failed to create session.uptime histogram: %w", err)
	}

	return nil
}

// sessionTracer tracks and records metrics for a specific Session lifecycle and operations.
type sessionTracer struct {
	mu          sync.Mutex
	startTime   time.Time
	openedAt    time.Time
	peerInfo    *spb.PeerInfo
	sessionName string
	sessionType SessionType
}

// newSessionTracer initializes a new sessionTracer starting the open timer.
func newSessionTracer(sessionType SessionType) *sessionTracer {
	return &sessionTracer{
		startTime:   time.Now(),
		sessionType: sessionType,
	}
}

func (t *sessionTracer) setPeerInfo(peerInfo *spb.PeerInfo, sessionName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peerInfo = peerInfo
	t.sessionName = sessionName
}

// recordOpen records the latency to open the session and starts the uptime timer.
func (t *sessionTracer) recordOpen(ctx context.Context, err error) {
	t.mu.Lock()
	t.openedAt = time.Now()
	pi := t.peerInfo
	name := t.sessionName
	t.mu.Unlock()

	if sessionOpenLatencies == nil {
		return
	}

	var transportType string
	var afeLocation string
	if pi != nil {
		transportType = pi.GetTransportType().String()
		afeLocation = pi.GetApplicationFrontendSubzone()
	}
	if transportType == "" {
		transportType = "unknown"
	}

	statusStr := "OK"
	if err != nil {
		statusStr = status.Code(err).String()
	}

	sessionOpenLatencies.Record(ctx, time.Since(t.startTime).Seconds()*1000.0, metric.WithAttributes(
		attribute.String("transport_type", transportType),
		attribute.String("status", statusStr),
		attribute.String("session_type", t.sessionType.String()),
		attribute.String("afe_location", afeLocation),
		attribute.String("session_name", name),
	))
}

// recordClose records the total lifetime/uptime of the session when it closes.
func (t *sessionTracer) recordClose(ctx context.Context) {
	t.mu.Lock()
	openedAt := t.openedAt
	pi := t.peerInfo
	name := t.sessionName
	t.mu.Unlock()

	if openedAt.IsZero() || sessionUptime == nil {
		return
	}

	var transportType string
	var afeLocation string
	if pi != nil {
		transportType = pi.GetTransportType().String()
		afeLocation = pi.GetApplicationFrontendSubzone()
	}
	if transportType == "" {
		transportType = "unknown"
	}

	sessionUptime.Record(ctx, time.Since(openedAt).Seconds()*1000.0, metric.WithAttributes(
		attribute.String("transport_type", transportType),
		attribute.String("session_type", t.sessionType.String()),
		attribute.Bool("ready", true),
		attribute.String("afe_location", afeLocation),
		attribute.String("session_name", name),
	))
}

// recordOperation records the execution duration of a virtual RPC within the session.
func (t *sessionTracer) recordOperation(ctx context.Context, opStartTime time.Time, method string, err error) {
	if sessionDurations == nil {
		return
	}

	t.mu.Lock()
	pi := t.peerInfo
	name := t.sessionName
	t.mu.Unlock()

	var transportType string
	var afeLocation string
	if pi != nil {
		transportType = pi.GetTransportType().String()
		afeLocation = pi.GetApplicationFrontendSubzone()
	}
	if transportType == "" {
		transportType = "unknown"
	}

	statusStr := "OK"
	if err != nil {
		statusStr = status.Code(err).String()
	}

	sessionDurations.Record(ctx, time.Since(opStartTime).Seconds()*1000.0, metric.WithAttributes(
		attribute.String("transport_type", transportType),
		attribute.String("status", statusStr),
		attribute.String("vrpcs", method),
		attribute.String("session_type", t.sessionType.String()),
		attribute.String("closing_reason", ""),
		attribute.Bool("ready", true),
		attribute.String("afe_location", afeLocation),
		attribute.String("session_name", name),
	))
}
