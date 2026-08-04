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

// outlierz view — the outlier-detection state per session pool. Surfaces
// which scorer is plugged in (name + config knobs), the current per-AFE
// cost multipliers, and the audit ring of recent score transitions
// (penalized / recovered). Complements loadz (picker decisions) with
// the upstream signal that shapes them.

package debugview

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

func newOutlierzHandler(p bigtable.OutlierDebugProvider) http.Handler {
	mux := http.NewServeMux()
	srv := &outlierzServer{provider: p}
	mux.HandleFunc("/", srv.handleIndex)
	return mux
}

type outlierzServer struct {
	provider bigtable.OutlierDebugProvider
}

// outlierzPage is the top-level template payload.
type outlierzPage struct {
	Generated time.Time
	Available bool
	Pools     []outlierzPoolView
}

// outlierzPoolView is one per-pool block. JSON-tagged so ?format=json
// returns a stable machine-readable shape.
type outlierzPoolView struct {
	PoolName       string            `json:"pool"`
	ScorerName     string            `json:"scorer"`
	Params         []paramRow        `json:"params"`
	Scores         []scoreRow        `json:"scores"`
	Recent         []recentRow       `json:"recent"`
	CapturedAt     time.Time         `json:"capturedAt"`
	// PenalizedCount is len(Scores where Penalized) — precomputed here
	// so the template doesn't need an inline count.
	PenalizedCount int `json:"penalizedCount"`
	TotalScored    int `json:"totalScored"`
}

type paramRow struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type scoreRow struct {
	AfeID     int64   `json:"afeId"`
	AfeIDHex  string  `json:"afeIdHex"`
	Score     float64 `json:"score"`
	Penalized bool    `json:"penalized"`
}

type recentRow struct {
	When         time.Time `json:"when"`
	AfeID        int64     `json:"afeId"`
	AfeIDHex     string    `json:"afeIdHex"`
	OldScore     float64   `json:"oldScore"`
	NewScore     float64   `json:"newScore"`
	Direction    string    `json:"direction"` // "penalized" | "recovered"
	SignalNanos  int64     `json:"signalNanos"`
	CohortNanos  int64     `json:"cohortMedianNanos"`
	Reason       string    `json:"reason"`
}

func (s *outlierzServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	views, avail := s.collect(now)

	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, struct {
			CapturedAt time.Time          `json:"capturedAt"`
			Available  bool               `json:"available"`
			Pools      []outlierzPoolView `json:"pools"`
		}{now, avail, views})
		return
	}
	writeHTML(w, outlierzTpl, outlierzPage{Generated: now, Available: avail, Pools: views})
}

// collect assembles per-pool views from the provider's snapshots.
// available reflects whether the provider itself was wired — false
// means EnableDebug=false on the underlying Client, and the page
// renders a "not enabled" banner.
func (s *outlierzServer) collect(now time.Time) ([]outlierzPoolView, bool) {
	if s.provider == nil {
		return nil, false
	}
	snaps := s.provider.Snapshot()
	views := make([]outlierzPoolView, 0, len(snaps))
	for _, snap := range snaps {
		views = append(views, buildOutlierzPoolView(snap, now))
	}
	return views, true
}

func buildOutlierzPoolView(snap btransport.OutlierPoolSnapshot, now time.Time) outlierzPoolView {
	params := make([]paramRow, 0, len(snap.Params))
	for _, p := range snap.Params {
		params = append(params, paramRow{Name: p.Name, Value: p.Value})
	}
	scores := make([]scoreRow, 0, len(snap.Scores))
	var penalized int
	for _, r := range snap.Scores {
		if r.Penalized {
			penalized++
		}
		scores = append(scores, scoreRow{
			AfeID:     int64(r.AfeID),
			AfeIDHex:  fmt.Sprintf("%x", uint64(r.AfeID)),
			Score:     r.Score,
			Penalized: r.Penalized,
		})
	}
	// Audit ring stored oldest-first; render newest-first.
	recent := make([]recentRow, 0, len(snap.Recent))
	for i := len(snap.Recent) - 1; i >= 0; i-- {
		d := snap.Recent[i]
		dir := "penalized"
		if d.NewScore <= d.OldScore {
			dir = "recovered"
		}
		recent = append(recent, recentRow{
			When:        d.When,
			AfeID:       int64(d.AfeID),
			AfeIDHex:    fmt.Sprintf("%x", uint64(d.AfeID)),
			OldScore:    d.OldScore,
			NewScore:    d.NewScore,
			Direction:   dir,
			SignalNanos: int64(d.Signal),
			CohortNanos: int64(d.CohortMedian),
			Reason:      d.Reason,
		})
	}
	capturedAt := snap.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = now
	}
	return outlierzPoolView{
		PoolName:       snap.PoolName,
		ScorerName:     snap.ScorerName,
		Params:         params,
		Scores:         scores,
		Recent:         recent,
		CapturedAt:     capturedAt,
		PenalizedCount: penalized,
		TotalScored:    len(scores),
	}
}

func outlierzFuncs() template.FuncMap {
	m := commonFuncs()
	m["dur"] = func(ns int64) string {
		if ns == 0 {
			return "—"
		}
		return roundDurationShort(time.Duration(ns)).String()
	}
	m["timeHM"] = func(t time.Time) string {
		return t.Format("15:04:05.000")
	}
	m["ago"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return roundDurationShort(time.Since(t)).String() + " ago"
	}
	m["scoreFmt"] = func(v float64) string {
		if v == 1.0 {
			return "1.00×"
		}
		return fmt.Sprintf("%.2f×", v)
	}
	m["hex"] = func(v int64) string {
		if v == 0 {
			return "unknown"
		}
		return fmt.Sprintf("0x%x", uint64(v))
	}
	m["scoreClass"] = func(penalized bool) string {
		if penalized {
			return "penalized"
		}
		return "healthy"
	}
	m["dirClass"] = func(dir string) string {
		if dir == "penalized" {
			return "dir-penalized"
		}
		return "dir-recovered"
	}
	return m
}

var outlierzTpl = template.Must(template.New("outlierz").Funcs(outlierzFuncs()).Parse(outlierzTplSrc))

const outlierzTplSrc = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="3">
<title>outlierz</title>
<style>
body { font-family: -apple-system,Segoe UI,Roboto,sans-serif; margin: 1rem; color: #222; }
h1 { font-size: 1.15rem; margin: 0 0 .5rem; }
h2 { font-size: 1rem; margin: 1.2rem 0 .2rem; color: #333; }
h3 { font-size: .88rem; margin: .8rem 0 .2rem; color: #555; font-weight: 600; text-transform: uppercase; letter-spacing: .04em; }
.gloss { color: #666; font-size: .85rem; margin: 0 0 .4rem; font-style: italic; }
table { border-collapse: collapse; width: 100%; font-size: .82rem; margin-bottom: .3rem; }
th, td { padding: .28rem .5rem; text-align: right; border-bottom: 1px solid #eee; font-variant-numeric: tabular-nums; }
th:first-child, td:first-child { text-align: left; }
th { background: #f6f6f6; font-weight: 600; }
.empty { color: #888; margin: 1rem 0; }
.gen { color: #888; font-size: .78rem; margin-top: 1rem; }
.scorer { color: #555; font-family: monospace; }
.summary { color: #666; font-size: .82rem; }
.healthy { color: #4a5; }
.penalized { color: #c33; font-weight: 700; }
.dir-penalized { color: #c33; font-weight: 600; }
.dir-recovered { color: #4a5; font-weight: 600; }
.reason { color: #555; font-size: .78rem; }
.disabled { background: #fff8dc; border-left: 4px solid #d4a017; padding: .8em 1em; margin: 0 0 1.2em 0; max-width: 44em; font-size: .92em; color: #5a4a10; }
.disabled code { background: #fff2c8; padding: 1px 4px; border-radius: 2px; }
.params { max-width: 34em; }
</style>
</head><body>
<h1>Bigtable outlier detection — {{if .Available}}{{len .Pools}} pool(s){{else}}not enabled{{end}}</h1>
{{if not .Available}}
<div class="disabled">
<strong>outlier debug not enabled.</strong> Per-pool outlier-scorer state
is not being collected. Set <code>bigtable.ClientConfig.EnableDebug = true</code>
and (optionally) plug a real scorer via
<code>SessionPoolImpl.SetOutlierScorer(NewLatencyOutlierScorer(...))</code>
before <code>Pool.Start</code>. Without a scorer the pool runs
<code>NoopScorer</code> — every AFE gets a 1.00× multiplier and the picker
runs as if this framework didn't exist.
</div>
{{end}}
{{range .Pools}}
<h2>{{.PoolName}} — <span class="scorer">{{.ScorerName}}</span>
  <span class="summary">· {{.PenalizedCount}}/{{.TotalScored}} AFEs penalized</span>
</h2>

<h3>Config</h3>
{{if .Params}}
<table class="params">
  <thead><tr><th>knob</th><th>value</th></tr></thead>
  <tbody>
  {{range .Params}}
  <tr><td>{{.Name}}</td><td>{{.Value}}</td></tr>
  {{end}}
  </tbody>
</table>
{{else}}
<p class="empty">Scorer exposes no configuration (typical for <code>noop</code>).</p>
{{end}}

<h3>Current per-AFE scores</h3>
{{if .Scores}}
<table>
  <thead><tr>
    <th>AFE id</th><th>score</th><th>status</th>
  </tr></thead>
  <tbody>
  {{range .Scores}}
  <tr>
    <td>{{hex .AfeID}}</td>
    <td class="{{scoreClass .Penalized}}">{{scoreFmt .Score}}</td>
    <td class="{{scoreClass .Penalized}}">{{if .Penalized}}penalized{{else}}healthy{{end}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{else}}
<p class="empty">No scores recorded yet — scorer has not run its first tick, or is NoopScorer.</p>
{{end}}

<h3>Recent transitions — last {{len .Recent}}</h3>
{{if .Recent}}
<table>
  <thead><tr>
    <th>time</th><th>AFE</th><th>direction</th><th>old → new</th><th>signal</th><th>cohort median</th><th>reason</th>
  </tr></thead>
  <tbody>
  {{range .Recent}}
  <tr>
    <td>{{timeHM .When}}</td>
    <td>{{hex .AfeID}}</td>
    <td class="{{dirClass .Direction}}">{{.Direction}}</td>
    <td>{{scoreFmt .OldScore}} → {{scoreFmt .NewScore}}</td>
    <td>{{dur .SignalNanos}}</td>
    <td>{{dur .CohortNanos}}</td>
    <td class="reason">{{.Reason}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{else}}
<p class="empty">No score transitions recorded yet.</p>
{{end}}

<p class="summary">captured {{ago .CapturedAt}}</p>
{{end}}
<p class="gen">Generated {{.Generated.Format "15:04:05.000 MST"}} — auto-refresh 3s.</p>
</body></html>`
