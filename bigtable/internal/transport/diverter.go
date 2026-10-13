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
	"math"
	"math/rand/v2"
	"strings"
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
	ScopeMethodMutateRows         = "method:Bigtable.MutateRows"
)

// isSupportedMethodScope reports whether a scope key from
// SessionDiversionConfiguration.PerScopeDiversion.ScopeDiversions is a
// single-dimension "method:..." scope. Other scope dimensions (or composite
// keys containing commas) are currently unsupported and ignored.
func isSupportedMethodScope(scope string) bool {
	if !strings.HasPrefix(scope, "method:") || strings.Contains(scope, ",") {
		return false
	}
	return len(strings.TrimPrefix(scope, "method:")) > 0
}

// normalizeMethodScope canonicalizes a method name or scope key (such as
// "ReadRow", "Bigtable.ReadRow", or "method:Bigtable.ReadRow") into the
// canonical "method:Bigtable.<Method>" form.
func normalizeMethodScope(scopeOrMethod string) string {
	if scopeOrMethod == "" {
		return ""
	}
	method := strings.TrimPrefix(scopeOrMethod, "method:")
	if !strings.Contains(method, ".") {
		return "method:Bigtable." + method
	}
	return "method:" + method
}

// Diverter decides whether to use the session-based protocol or classic protocol.
// It supports both a uniform/global session load and per-scope ("method")
// session loads configured via SessionDiversionConfiguration.
type Diverter struct {
	sessionLoadBits atomic.Uint64
	// scopeLoads holds an immutable map from canonical method scope
	// ("method:Bigtable.<Method>") to session load ratio [0.0, 1.0].
	// When nil, all methods use the uniform load stored in sessionLoadBits.
	scopeLoads atomic.Pointer[map[string]float64]
	// sessionPicks / classicPicks tally every UseSession* outcome so the
	// debug UI can report the actual ratio of traffic landing on each path
	// — useful when the configured SessionLoad doesn't match what shipped
	// (e.g. mid-rollout or after a control-plane override).
	sessionPicks atomic.Int64
	classicPicks atomic.Int64
}

// NewDiverter creates a new Diverter with the given uniform session load ratio (0.0 to 1.0).
func NewDiverter(sessionLoad float64) *Diverter {
	d := &Diverter{}
	d.sessionLoadBits.Store(math.Float64bits(sessionLoad))
	return d
}

// SetSessionLoad updates the uniform session load ratio across all scopes,
// clearing any per-scope overrides.
func (d *Diverter) SetSessionLoad(load float64) {
	d.scopeLoads.Store(nil)
	d.sessionLoadBits.Store(math.Float64bits(load))
}

// SetScopeSessionLoads sets per-scope session loads for the Diverter. Keys
// are normalized to "method:Bigtable.<Method>". Any method not present in the
// map defaults to 0.0 session load.
func (d *Diverter) SetScopeSessionLoads(loads map[string]float64) {
	copied := make(map[string]float64, len(loads))
	for k, v := range loads {
		if isSupportedMethodScope(k) || !strings.Contains(k, ":") {
			copied[normalizeMethodScope(k)] = v
		}
	}
	d.scopeLoads.Store(&copied)
	d.sessionLoadBits.Store(math.Float64bits(copied[ScopeMethodReadRow]))
}

// ApplySessionClientConfiguration updates the Diverter according to the
// server-supplied SessionClientConfiguration following the session.proto
// precedence rules:
//  1. Apply the deprecated top-level session_load field to Bigtable.ReadRow
//     and Bigtable.MutateRow (all other methods default to 0.0).
//  2. Process session_diversion_configuration, overwriting any behavior
//     established in step 1:
//     - global_diversion sets a uniform load across all methods.
//     - per_scope_diversion sets per-method loads for each supported
//       "method:..." scope key in scope_diversions.
func (d *Diverter) ApplySessionClientConfiguration(cfg *bigtablepb.SessionClientConfiguration) {
	if cfg == nil {
		d.SetSessionLoad(0)
		return
	}
	loads := make(map[string]float64)
	if baseLoad := float64(cfg.GetSessionLoad()); baseLoad != 0 {
		loads[ScopeMethodReadRow] = baseLoad
		loads[ScopeMethodMutateRow] = baseLoad
	}
	if divCfg := cfg.GetSessionDiversionConfiguration(); divCfg != nil {
		switch strategy := divCfg.GetDiversionStrategy().(type) {
		case *bigtablepb.SessionDiversionConfiguration_GlobalDiversion:
			d.SetSessionLoad(float64(strategy.GlobalDiversion.GetSessionLoad()))
			return
		case *bigtablepb.SessionDiversionConfiguration_PerScopeDiversion_:
			for scope, scopeCfg := range strategy.PerScopeDiversion.GetScopeDiversions() {
				if isSupportedMethodScope(scope) {
					loads[normalizeMethodScope(scope)] = float64(scopeCfg.GetSessionLoad())
				}
			}
		}
	}
	d.scopeLoads.Store(&loads)
	d.sessionLoadBits.Store(math.Float64bits(loads[ScopeMethodReadRow]))
}

// SessionLoad returns the current target session-protocol fraction (or the
// ReadRow session load when per-scope diversion is active).
func (d *Diverter) SessionLoad() float64 {
	return math.Float64frombits(d.sessionLoadBits.Load())
}

// SessionLoadForMethod returns the current target session-protocol fraction
// for the given method or scope key (e.g. "ReadRow", "Bigtable.ReadRow", or
// "method:Bigtable.ReadRow").
func (d *Diverter) SessionLoadForMethod(method string) float64 {
	if m := d.scopeLoads.Load(); m != nil {
		return (*m)[normalizeMethodScope(method)]
	}
	return math.Float64frombits(d.sessionLoadBits.Load())
}

// ScopeSessionLoads returns a copy of the active per-scope session load map,
// or nil when the Diverter is operating with a uniform global session load.
func (d *Diverter) ScopeSessionLoads() map[string]float64 {
	m := d.scopeLoads.Load()
	if m == nil {
		return nil
	}
	copied := make(map[string]float64, len(*m))
	for k, v := range *m {
		copied[k] = v
	}
	return copied
}

// UseSession returns true if the next call should use the session protocol
// under the Diverter's global/default session load.
func (d *Diverter) UseSession() bool {
	return d.pickWithLoad(d.SessionLoad())
}

// UseSessionForMethod returns true if the next call for the given method
// (e.g. "ReadRow", "Bigtable.MutateRow", or "method:Bigtable.MutateRows")
// should use the session protocol.
func (d *Diverter) UseSessionForMethod(method string) bool {
	return d.pickWithLoad(d.SessionLoadForMethod(method))
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
	SessionLoad  float64
	ScopeLoads   map[string]float64
	SessionPicks int64
	ClassicPicks int64
}

// Snapshot returns the diverter's current target SessionLoad (and any
// per-scope loads) plus the running totals of pick outcomes.
func (d *Diverter) Snapshot() DiverterSnapshot {
	return DiverterSnapshot{
		SessionLoad:  d.SessionLoad(),
		ScopeLoads:   d.ScopeSessionLoads(),
		SessionPicks: d.sessionPicks.Load(),
		ClassicPicks: d.classicPicks.Load(),
	}
}
