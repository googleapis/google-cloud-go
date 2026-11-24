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
	"math/rand/v2"
	"sync/atomic"

	bigtablepb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// Supported method scope keys for per-scope session diversion.
// See SessionDiversionConfiguration.PerScopeDiversion in session.proto.
const (
	ScopeMethodReadRow            = "method:Bigtable.ReadRow"
	ScopeMethodMutateRow          = "method:Bigtable.MutateRow"
	ScopeMethodCheckAndMutateRow  = "method:Bigtable.CheckAndMutateRow"
	ScopeMethodReadModifyWriteRow = "method:Bigtable.ReadModifyWriteRow"
)

// servedMethodScopes enumerates the exact method scopes this client serves via
// sessions. Sharing this list and resolveScopeSessionLoads between Diverter and
// ClientConfigurationManager guarantees that per-RPC diversion and client-wide
// "are sessions in use" checks always agree (unserved scopes cannot keep
// session machinery or non-zero debug tags active while diverting zero RPCs).
var servedMethodScopes = []string{
	ScopeMethodReadRow,
	ScopeMethodMutateRow,
	ScopeMethodCheckAndMutateRow,
	ScopeMethodReadModifyWriteRow,
}

// resolveScopeSessionLoads resolves the effective session load ratio for each
// served method scope from a server-supplied SessionClientConfiguration using
// SessionDiversionConfiguration exclusively (deprecated SessionLoad is
// ignored). Absent, empty, or unrecognized diversion strategies resolve to 0
// across all served scopes.
func resolveScopeSessionLoads(sc *bigtablepb.SessionClientConfiguration) map[string]float64 {
	loads := make(map[string]float64, len(servedMethodScopes))
	divCfg := sc.GetSessionDiversionConfiguration()
	if divCfg == nil {
		for _, scope := range servedMethodScopes {
			loads[scope] = 0
		}
		return loads
	}
	switch strategy := divCfg.GetDiversionStrategy().(type) {
	case *bigtablepb.SessionDiversionConfiguration_GlobalDiversion:
		load := float64(strategy.GlobalDiversion.GetSessionLoad())
		for _, scope := range servedMethodScopes {
			loads[scope] = load
		}
	case *bigtablepb.SessionDiversionConfiguration_PerScopeDiversion_:
		scopeDiversions := strategy.PerScopeDiversion.GetScopeDiversions()
		for _, scope := range servedMethodScopes {
			var load float64
			if scopeCfg := scopeDiversions[scope]; scopeCfg != nil {
				load = float64(scopeCfg.GetSessionLoad())
			}
			loads[scope] = load
		}
	default:
		for _, scope := range servedMethodScopes {
			loads[scope] = 0
		}
	}
	return loads
}

// Diverter decides whether to use the session-based protocol or classic protocol
// for each served method scope ("method:Bigtable.<Method>").
type Diverter struct {
	// scopeLoads holds an immutable map from canonical method scope
	// ("method:Bigtable.<Method>") to session load ratio [0.0, 1.0].
	scopeLoads atomic.Pointer[map[string]float64]
	// sessionPicks / classicPicks tally every UseSessionForMethod outcome so
	// the debug UI can report the actual ratio of traffic landing on each path
	// — useful when the configured SessionLoad doesn't match what shipped
	// (e.g. mid-rollout or after a control-plane override).
	sessionPicks atomic.Int64
	classicPicks atomic.Int64
}

// NewDiverter creates a new Diverter with 0.0 session load across all served
// method scopes until updated by ApplySessionClientConfiguration (or test
// setters).
func NewDiverter() *Diverter {
	d := &Diverter{}
	d.SetSessionLoad(0.0)
	return d
}

// SetSessionLoad updates the uniform session load ratio across all served
// method scopes.
func (d *Diverter) SetSessionLoad(load float64) {
	loads := make(map[string]float64, len(servedMethodScopes))
	for _, scope := range servedMethodScopes {
		loads[scope] = load
	}
	d.scopeLoads.Store(&loads)
}

// SetScopeSessionLoads sets per-scope session loads for the Diverter. Keys
// must be canonical served scope names ("method:Bigtable.<Method>"). Any
// served scope omitted from loads defaults to 0.0; unserved scope keys are
// ignored.
func (d *Diverter) SetScopeSessionLoads(loads map[string]float64) {
	copied := make(map[string]float64, len(servedMethodScopes))
	for _, scope := range servedMethodScopes {
		copied[scope] = loads[scope]
	}
	d.scopeLoads.Store(&copied)
}

// ApplySessionClientConfiguration updates the Diverter according to the
// server-supplied SessionClientConfiguration's SessionDiversionConfiguration
// (deprecated SessionLoad is ignored).
func (d *Diverter) ApplySessionClientConfiguration(cfg *bigtablepb.SessionClientConfiguration) {
	loads := resolveScopeSessionLoads(cfg)
	d.scopeLoads.Store(&loads)
}

// SessionLoadForMethod returns the current target session-protocol fraction
// for the given canonical method scope key (e.g. ScopeMethodReadRow).
func (d *Diverter) SessionLoadForMethod(scope string) float64 {
	m := d.scopeLoads.Load()
	if m == nil {
		return 0
	}
	return (*m)[scope]
}

// ScopeSessionLoads returns a copy of the active per-scope session load map
// across all served method scopes.
func (d *Diverter) ScopeSessionLoads() map[string]float64 {
	copied := make(map[string]float64, len(servedMethodScopes))
	m := d.scopeLoads.Load()
	for _, scope := range servedMethodScopes {
		if m != nil {
			copied[scope] = (*m)[scope]
		} else {
			copied[scope] = 0
		}
	}
	return copied
}

// UseSessionForMethod returns true if the next call for the given canonical
// method scope key (e.g. ScopeMethodReadRow) should use the session protocol.
func (d *Diverter) UseSessionForMethod(scope string) bool {
	return d.pickWithLoad(d.SessionLoadForMethod(scope))
}

func (d *Diverter) pickWithLoad(load float64) bool {
	var use bool
	switch {
	case load <= 0:
		use = false
	case load >= 1:
		use = true
	default:
		use = rand.Float64() <= load
	}
	if use {
		d.sessionPicks.Add(1)
	} else {
		d.classicPicks.Add(1)
	}
	return use
}

// DiverterSnapshot is what the debug UI surfaces for the diverter.
type DiverterSnapshot struct {
	ScopeLoads   map[string]float64
	SessionPicks int64
	ClassicPicks int64
}

// Snapshot returns the diverter's current per-scope target loads plus the
// running totals of pick outcomes.
func (d *Diverter) Snapshot() DiverterSnapshot {
	return DiverterSnapshot{
		ScopeLoads:   d.ScopeSessionLoads(),
		SessionPicks: d.sessionPicks.Load(),
		ClassicPicks: d.classicPicks.Load(),
	}
}
