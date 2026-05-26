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

package bigtable

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// SessionManager manages dynamic table-specific session pools for vRPC operations.
type SessionManager struct {
	mu                sync.Mutex
	enableSessionPool bool
	metricsEnabled    bool
	disableRetryInfo  bool
	featureFlagsMD    metadata.MD
	diverter          *btransport.Diverter
	configManager     *btransport.ClientConfigurationManager
	backgroundCtx     context.Context
	sessionPools      map[string]*btransport.SessionPoolImpl
	minSessions       int
	maxSessions       int
	channelPool       managedChannelPool
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(
	enableSessionPool bool,
	metricsEnabled bool,
	disableRetryInfo bool,
	featureFlagsMD metadata.MD,
	diverter *btransport.Diverter,
	configManager *btransport.ClientConfigurationManager,
	backgroundCtx context.Context,
	minSessions int,
	maxSessions int,
	meterProvider metric.MeterProvider,
	channelPool managedChannelPool,
) *SessionManager {
	if metricsEnabled && meterProvider != nil {
		_ = btransport.InitializeMetrics(meterProvider)
	}
	return &SessionManager{
		enableSessionPool: enableSessionPool,
		metricsEnabled:    metricsEnabled,
		disableRetryInfo:  disableRetryInfo,
		featureFlagsMD:    featureFlagsMD,
		diverter:          diverter,
		configManager:     configManager,
		backgroundCtx:     backgroundCtx,
		sessionPools:      make(map[string]*btransport.SessionPoolImpl),
		minSessions:       minSessions,
		maxSessions:       maxSessions,
		channelPool:       channelPool,
	}
}

// Close closes all session pools managed by the SessionManager.
func (m *SessionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pool := range m.sessionPools {
		pool.Close()
	}
	var err error
	if m.channelPool.pool != nil {
		err = m.channelPool.Close()
	}
	return err
}

// GetOrCreateSessionTable initializes or retrieves session-based transport pools for a table/view.
func (m *SessionManager) GetOrCreateSessionTable(
	resourceName string,
	classic *Table,
	sessionDesc *btransport.SessionDescriptor,
	readStreamFactory func(ctx context.Context) (btransport.Stream, error),
	writeStreamFactory func(ctx context.Context) (btransport.Stream, error),
	readPayload proto.Message,
	writePayload proto.Message,
	readVRpcDesc btransport.VRpcDescriptor,
	writeVRpcDesc btransport.VRpcDescriptor,
	keyPrefix string,
) TableAPI {
	if !m.enableSessionPool {
		return &tableImpl{*classic}
	}

	flags := &btpb.FeatureFlags{
		RoutingCookie:            true,
		ReverseScans:             true,
		LastScannedRowResponses:  true,
		ClientSideMetricsEnabled: m.metricsEnabled,
		RetryInfo:                !m.disableRetryInfo,
		TrafficDirectorEnabled:   true,
		DirectAccessRequested:    true,
		SessionsCompatible:       true,
		PeerInfo:                 true,
	}

	readKey := fmt.Sprintf("%s:read", keyPrefix)
	readPool := m.createPoolForPayload(resourceName, sessionDesc, readStreamFactory, readPayload, flags, readKey)

	writeKey := fmt.Sprintf("%s:write", keyPrefix)
	writePool := m.createPoolForPayload(resourceName, sessionDesc, writeStreamFactory, writePayload, flags, writeKey)

	if readPool != nil && m.diverter != nil {
		sessionTable := NewSessionTable(classic.table, classic, readPool, writePool, readVRpcDesc, writeVRpcDesc)
		return NewTableShim(&tableImpl{*classic}, sessionTable, m.diverter)
	}

	return &tableImpl{*classic}
}

func (m *SessionManager) createPoolForPayload(
	resourceName string,
	sessionDesc *btransport.SessionDescriptor,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	payload proto.Message,
	flags *btpb.FeatureFlags,
	key string,
) *btransport.SessionPoolImpl {
	if payload == nil {
		return nil
	}

	payloadBytes, _ := proto.Marshal(payload)
	handshake := &btpb.OpenSessionRequest{
		ProtocolVersion: 1,
		Payload:         payloadBytes,
		Flags:           flags,
	}

	var sessionMetadata []string
	for k, v := range sessionDesc.MetadataFn(payload) {
		sessionMetadata = append(sessionMetadata, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	paramsVal := strings.Join(sessionMetadata, "&")

	md := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, resourceName,
		requestParamsHeader, paramsVal,
	), m.featureFlagsMD)

	min := 10
	if m.minSessions > 0 {
		min = m.minSessions
	}
	max := 100
	if m.maxSessions > 0 {
		max = m.maxSessions
	}
	return m.GetOrCreateSessionPool(key, min, max, streamFactory, handshake, md, sessionDesc.Type)
}

// GetOrCreateSessionPool gets or creates a session pool for a specific key.
func (m *SessionManager) GetOrCreateSessionPool(
	key string,
	min, max int,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	openSessionRequest *btpb.OpenSessionRequest,
	md metadata.MD,
	sessionType btransport.SessionType,
) *btransport.SessionPoolImpl {
	fmt.Printf(">>> getOrCreateSessionPool: key=%s, min=%d, max=%d <<<\n", key, min, max)
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, ok := m.sessionPools[key]
	if !ok {
		pool = btransport.NewSessionPoolImpl(key, min, max, streamFactory, openSessionRequest, md, sessionType)
		m.sessionPools[key] = pool

		if m.configManager != nil {
			m.configManager.AddSessionPoolListener(func(config *btpb.SessionClientConfiguration_SessionPoolConfiguration) {
				pool.UpdateConfig(config)
			})
		}

		// Start background heartbeat scaling pacemaker loop for this dedicated pool!
		pool.StartHeartbeat(m.backgroundCtx, 1*time.Second)
		pool.PerformScaling(m.backgroundCtx)
	}
	return pool
}
