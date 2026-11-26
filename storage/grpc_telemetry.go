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

package storage

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

const (
	transportCloudPath              = "CloudPath"
	transportDirectPath             = "DirectPath"
	transportDirectPathInterconnect = "DirectPathInterconnect"

	channelReadyTimeout   = 30 * time.Second
	unknownTelemetryValue = "(unknown)"
	forceXDSQueryKey      = "force-xds"
)

// connectivityWatcher is the subset of *grpc.ClientConn used to observe channel
// readiness.
type connectivityWatcher interface {
	Target() string
	GetState() connectivity.State
	Connect()
	WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool
}

// startClientChannelTelemetry emits the startup transport configuration log and
// begins an asynchronous readiness watch on conn. It returns a stop function
// that cancels the watcher and waits for its goroutine to exit, or nil when
// telemetry is skipped (custom gRPC connection or INFO logging disabled).
func startClientChannelTelemetry(ctx context.Context, opts []option.ClientOption, conn *grpc.ClientConn, start time.Time) func() {
	if res, err := internaloption.NewUnsafeResolver(opts...); err == nil && res.ResolvedGRPCConnIsCustom() {
		return nil
	}
	logger := internaloption.GetLogger(opts)
	if !logger.Enabled(ctx, slog.LevelInfo) {
		return nil
	}
	return startChannelTelemetry(ctx, logger, conn, start)
}

func startChannelTelemetry(ctx context.Context, logger *slog.Logger, conn connectivityWatcher, start time.Time) func() {
	logChannelConfiguration(logger, conn.Target())
	return watchChannelReady(ctx, logger, conn, start, channelReadyTimeout)
}

// logChannelConfiguration emits the client's requested gRPC transport once per
// SDK-managed client construction. It logs requested_transport_type rather than
// transport_type because no channel has connected yet, and logs authority as
// (unknown) because grpc.WithAuthority dial options are opaque and
// grpc.ClientConn does not expose its effective authority.
func logChannelConfiguration(logger *slog.Logger, endpoint string) {
	logger.Info(
		"Configured gRPC transport.",
		"requested_transport_type", detectTransportType(endpoint),
		"endpoint", endpoint,
		"authority", unknownTelemetryValue,
		"grpc_version", grpc.Version,
	)
}

// watchChannelReady watches conn asynchronously until READY, SHUTDOWN, stop, or
// timeout. The watch context detaches cancellation from ctx (which callers
// often cancel as soon as NewGRPCClient returns) while preserving context
// values, and is cancelled immediately by the returned stop function on
// Client.Close.
func watchChannelReady(ctx context.Context, logger *slog.Logger, conn connectivityWatcher, start time.Time, timeout time.Duration) func() {
	requested := detectTransportType(conn.Target())
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Release the deadline timer immediately when the channel reaches READY
		// or SHUTDOWN, even if the client remains open for hours.
		defer cancel()

		for {
			if err := watchCtx.Err(); err != nil {
				logChannelReady(logger, requested, time.Since(start), err.Error())
				return
			}
			state := conn.GetState()
			switch state {
			case connectivity.Ready:
				logChannelReady(logger, requested, time.Since(start), "")
				return
			case connectivity.Shutdown:
				logChannelReady(logger, requested, time.Since(start), "channel shut down")
				return
			case connectivity.Idle:
				// Safeguard on lazy/idle channels so readiness reflects channel
				// setup rather than waiting for the first RPC.
				conn.Connect()
			}
			if !conn.WaitForStateChange(watchCtx, state) {
				status := "watch stopped"
				if err := watchCtx.Err(); err != nil {
					status = err.Error()
				}
				logChannelReady(logger, requested, time.Since(start), status)
				return
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func logChannelReady(logger *slog.Logger, requested string, elapsed time.Duration, failureStatus string) {
	elapsedMS := max(elapsed.Milliseconds(), 0)
	if failureStatus != "" {
		// Short-lived clients closed before connecting also reach this path, so
		// log at INFO and omit transport_type.
		logger.Info(
			"gRPC channel [0] did not become ready.",
			"requested_transport_type", requested,
			"lb_policy", unknownTelemetryValue,
			"elapsed_ms", elapsedMS,
			"status", failureStatus,
		)
		return
	}
	msg := "gRPC channel [0] is ready" + channelReadyNote(requested) + "."
	logger.Info(
		msg,
		"transport_type", requested,
		"requested_transport_type", requested,
		"lb_policy", unknownTelemetryValue,
		"elapsed_ms", elapsedMS,
	)
}

// detectTransportType classifies the channel transport from a dialed gRPC
// target URI. It requires the three-slash google-c2p:/// form produced by
// auth/grpctransport (case-insensitive per RFC 3986), and mirrors grpc-go's
// google-c2p resolver by checking key presence of "force-xds" in the parsed
// URL query to identify DirectPath over Interconnect.
func detectTransportType(target string) string {
	if !strings.HasPrefix(strings.ToLower(target), directPathEndpointPrefix) {
		return transportCloudPath
	}
	u, err := url.Parse(target)
	if err != nil {
		return transportCloudPath
	}
	if u.Query().Has(forceXDSQueryKey) {
		return transportDirectPathInterconnect
	}
	return transportDirectPath
}

func channelReadyNote(requested string) string {
	if requested != transportCloudPath {
		return ", but gRPC does not expose the LB policy, so transport_type assumes the requested transport"
	}
	return ""
}
