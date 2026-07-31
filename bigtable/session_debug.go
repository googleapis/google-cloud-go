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
// Clients where sessionImpl is nil) or when session-side debug
// recorders are disabled.
//
// The concrete provider layers the mixed-mode Diverter on top of the
// session client's own session-only provider. sessionImpl.SessionDebug
// intentionally returns an empty DiverterSnapshot (per its doc, "only
// bigtable.Client knows about the classic/session split"); the adapter
// below overrides Diverter() with the Client's actual Diverter.Snapshot.
func (c *Client) SessionDebug() SessionDebugProvider {
	if c.sessionImpl == nil {
		return nil
	}
	base := c.sessionImpl.SessionDebug()
	if base == nil {
		return nil
	}
	return mixedModeSessionDebug{base: base, diverter: c.diverter}
}

// mixedModeSessionDebug wraps a session-only provider with the Client's
// classic/session Diverter. Snapshot + LoadBalancingSnapshots pass
// through unchanged; only Diverter() overrides.
type mixedModeSessionDebug struct {
	base     SessionDebugProvider
	diverter *btransport.Diverter
}

func (m mixedModeSessionDebug) Snapshot() []btransport.PoolSnapshot {
	return m.base.Snapshot()
}

func (m mixedModeSessionDebug) LoadBalancingSnapshots() []btransport.LoadBalancingSnapshot {
	return m.base.LoadBalancingSnapshots()
}

func (m mixedModeSessionDebug) Diverter() btransport.DiverterSnapshot {
	if m.diverter == nil {
		return btransport.DiverterSnapshot{}
	}
	return m.diverter.Snapshot()
}

// ChannelDebug returns a ChannelDebugProvider for this Client. Emits
// exactly one entry for the classic channel pool (Role="classic") and
// prepends the session channel pool entries when the session backend
// is wired.
func (c *Client) ChannelDebug() ChannelDebugProvider {
	return mixedModeChannelDebug{client: c}
}

// mixedModeChannelDebug composes the Client's classic channel pool
// with the session pool contributed by sessionImpl. Emits the classic
// entry first (Role="classic"), then the session entry (Role="session")
// when session pooling is enabled and its pool is a
// *btransport.BigtableChannelPool (skipped otherwise — test fakes may
// substitute a non-BigtableChannelPool).
type mixedModeChannelDebug struct {
	client *Client
}

func (a mixedModeChannelDebug) Snapshot() []ChannelPoolDebug {
	out := make([]ChannelPoolDebug, 0, 2)
	instance := a.client.fullInstanceName()
	if p := bigtableChannelPool(a.client.mPool.Pool); p != nil {
		out = append(out, ChannelPoolDebug{
			Role:         "classic",
			Snapshot:     p.ChannelPoolSnapshot(),
			InstanceName: instance,
			AppProfile:   a.client.appProfile,
		})
	}
	if a.client.sessionImpl != nil {
		if sp := a.client.sessionImpl.ChannelDebug(); sp != nil {
			// Session-side entries come pre-labeled with Role="session";
			// stamp instance / app-profile onto each since the session
			// package can't reach back to the outer bigtable.Client.
			for _, e := range sp.Snapshot() {
				if e.InstanceName == "" {
					e.InstanceName = instance
				}
				if e.AppProfile == "" {
					e.AppProfile = a.client.appProfile
				}
				out = append(out, e)
			}
		}
	}
	return out
}

// bigtableChannelPool extracts *btransport.BigtableChannelPool from a
// gtransport.ConnPool, returning nil if the pool isn't a
// BigtableChannelPool (e.g. an emulator-only client that uses the
// simple gtransport.DialPool).
func bigtableChannelPool(p interface{}) *btransport.BigtableChannelPool {
	if p == nil {
		return nil
	}
	bp, _ := p.(*btransport.BigtableChannelPool)
	return bp
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
// returned pointer is safe to hand to bigtable/debugview.Handler for
// the tcpz page.
func (c *Client) TCPStats() *TCPStats {
	return c.tcpStats
}
