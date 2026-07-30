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
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// The provider interface + struct types live in internal/transport so
// internal/session.Client can implement them without an import cycle.
// Re-exported here as type aliases — external names + identity unchanged.

// SessionDebugProvider exposes a snapshot of every session pool the
// Client currently owns.
type SessionDebugProvider = btransport.SessionDebugProvider

// ChannelDebugProvider exposes a snapshot of every gRPC channel pool
// the Client currently owns.
type ChannelDebugProvider = btransport.ChannelDebugProvider

// ChannelPoolDebug names a single channel pool and carries its snapshot.
type ChannelPoolDebug = btransport.ChannelPoolDebug

// SessionRef identifies one session for the channelz → sessionz reverse
// link.
type SessionRef = btransport.SessionRef

// ConfigDebugProvider exposes a snapshot of the most recent
// GetClientConfiguration poll outcome.
type ConfigDebugProvider = btransport.ConfigDebugProvider

// SessionDebug returns a SessionDebugProvider for this Client. Returns
// nil when the session backend isn't wired (hand-built or emulator-only
// Clients where sessionImpl is nil) or when the session client's own
// debug recorders are disabled. The debugview handler renders the
// "not enabled" panel in that case.
func (c *Client) SessionDebug() SessionDebugProvider {
	if c.sessionImpl == nil {
		return nil
	}
	return c.sessionImpl.SessionDebug()
}

// ChannelDebug returns a ChannelDebugProvider for this Client. Returns
// nil when the session backend isn't wired. When wired, delegates to
// sessionImpl.ChannelDebug — which contributes entries for the session
// channel pool. The classic-side pool is not surfaced here today; if
// that becomes needed, wrap this in a mixed-mode adapter that adds a
// classic entry from c.mPool.
func (c *Client) ChannelDebug() ChannelDebugProvider {
	if c.sessionImpl == nil {
		return nil
	}
	return c.sessionImpl.ChannelDebug()
}

// ConfigDebug returns a ConfigDebugProvider for this Client. Returns
// nil when the session backend isn't wired (no configuration manager
// is constructed in that mode).
func (c *Client) ConfigDebug() ConfigDebugProvider {
	if c.sessionImpl == nil {
		return nil
	}
	return c.sessionImpl.ConfigDebug()
}

// TCPStats returns the per-connection TCP_INFO collector when
// ClientConfig.EnableDebug was set at construction, else nil. The
// returned pointer is safe to hand directly to
// bigtable/debugview.Handler for the tcpz page.
func (c *Client) TCPStats() *TCPStats {
	return c.tcpStats
}
