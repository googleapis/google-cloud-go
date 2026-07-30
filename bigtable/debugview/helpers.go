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

package debugview

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// writeJSON is the shared JSON response helper used by every -z view. All
// responders (afez/loadz/sessionz) used byte-identical bodies before the
// fold; channelz/configz inlined the same logic. Kept here so tweaks
// (e.g. gzip, streaming) can land in one place.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeHTML is the shared HTML response helper used by every -z view.
func writeHTML(w http.ResponseWriter, tpl *template.Template, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// roundDurationShort is the sub-second-first rounding rule used by
// latency-oriented columns (afez / loadz EWMAs and ages). Preserves
// microsecond resolution for sub-millisecond values and drops to 10ms
// buckets once you're past a second.
func roundDurationShort(d time.Duration) time.Duration {
	switch {
	case d < 0:
		return -roundDurationShort(-d)
	case d == 0:
		return 0
	case d < time.Millisecond:
		return d.Round(time.Microsecond)
	case d < time.Second:
		return d.Round(time.Millisecond)
	default:
		return d.Round(10 * time.Millisecond)
	}
}

// roundDurationLong is the wall-clock-first rounding rule used by
// "age since" columns (channelz / configz / sessionz). Coarsens up to
// minutes/seconds for the multi-minute values that dominate those views.
func roundDurationLong(d time.Duration) time.Duration {
	switch {
	case d > time.Hour:
		return d.Round(time.Minute)
	case d > time.Minute:
		return d.Round(time.Second)
	case d > time.Second:
		return d.Round(10 * time.Millisecond)
	default:
		return d.Round(time.Microsecond)
	}
}

// peerShort renders the AFE routing tuple (id-in-hex / region / subzone)
// used by sessionz's slow-vRPC log Peer column.
//
// Returns "" when the AFE header hasn't landed yet; substitutes "?" for
// empty region / subzone (sessionz's readability tweak). Callers who
// need a visible placeholder for the empty case should inline
// `{{else}}—{{end}}` at their template call site.
func peerShort(p btransport.PeerInfoSnapshot) string {
	if p.ApplicationFrontendID == 0 && p.ApplicationFrontendRegion == "" && p.ApplicationFrontendSubzone == "" {
		return ""
	}
	region := p.ApplicationFrontendRegion
	if region == "" {
		region = "?"
	}
	subzone := p.ApplicationFrontendSubzone
	if subzone == "" {
		subzone = "?"
	}
	return fmt.Sprintf("%x/%s/%s", p.ApplicationFrontendID, region, subzone)
}

// commonFuncs returns a fresh FuncMap seeded with the entries that
// overlap across two or more views. Each view calls this then adds its
// own view-specific keys on top. Kept as a fresh copy (rather than a
// package-level shared map) so per-view templates can safely override any
// key without cross-contamination.
func commonFuncs() template.FuncMap {
	return template.FuncMap{
		"orDash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		"timestamp": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.Format(time.RFC3339)
		},
		"boolMark": func(b bool) string {
			if b {
				return "✓"
			}
			return ""
		},
	}
}
