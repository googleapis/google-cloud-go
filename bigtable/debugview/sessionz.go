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

// sessionz view — per-pool session state, latency histograms, slow-vRPC
// log, and scaling history. The most detailed view; pulls live state on
// each request with no background goroutines and no per-RPC overhead
// beyond two atomic increments.

package debugview

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

func newSessionzHandler(p bigtable.SessionDebugProvider) http.Handler {
	mux := http.NewServeMux()
	srv := &sessionzServer{provider: p}
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/pool/", srv.handlePool)
	return mux
}

type sessionzServer struct {
	provider bigtable.SessionDebugProvider
}

func (s *sessionzServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	snaps := s.snapshot()
	var diverter btransport.DiverterSnapshot
	if s.provider != nil {
		diverter = s.provider.Diverter()
	}
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, struct {
			Pools    []btransport.PoolSnapshot   `json:"pools"`
			Diverter btransport.DiverterSnapshot `json:"diverter"`
		}{snaps, diverter})
		return
	}
	writeHTML(w, sessionzIndexTpl, sessionzIndexData{
		Pools:       snaps,
		Diverter:    diverter,
		Generated:   time.Now(),
		HasProvider: s.provider != nil,
	})
}

func (s *sessionzServer) handlePool(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/pool/")
	if key == "" {
		http.NotFound(w, r)
		return
	}
	snaps := s.snapshot()
	var found *btransport.PoolSnapshot
	for i := range snaps {
		if snaps[i].Name == key {
			found = &snaps[i]
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, found)
		return
	}
	writeHTML(w, sessionzPoolTpl, sessionzPoolData{
		Pool:      *found,
		Generated: time.Now(),
	})
}

func (s *sessionzServer) snapshot() []btransport.PoolSnapshot {
	if s.provider == nil {
		return nil
	}
	return s.provider.Snapshot()
}

type sessionzIndexData struct {
	Pools       []btransport.PoolSnapshot
	Diverter    btransport.DiverterSnapshot
	Generated   time.Time
	HasProvider bool
}

type sessionzPoolData struct {
	Pool      btransport.PoolSnapshot
	Generated time.Time
}

func sessionzFuncs() template.FuncMap {
	m := commonFuncs()
	m["age"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return roundDurationLong(time.Since(t)).String()
	}
	m["untilNow"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		d := time.Until(t)
		if d < 0 {
			return "expired " + roundDurationLong(-d).String() + " ago"
		}
		return "in " + roundDurationLong(d).String()
	}
	m["dur"] = func(d time.Duration) string {
		if d == 0 {
			return "—"
		}
		return roundDurationLong(d).String()
	}
	m["stateClass"] = func(state string) string {
		switch state {
		case "Ready":
			return "state-active"
		case "Starting", "New":
			return "state-starting"
		case "Closing", "WaitServerClose":
			return "state-closing"
		case "Closed":
			return "state-closed"
		}
		return ""
	}
	m["signed"] = func(n int) string {
		if n > 0 {
			return "+" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	m["reverseSlow"] = func(s []btransport.SlowVRpcEvent) []btransport.SlowVRpcEvent {
		// Operators want newest-first in a slow log — the most recent
		// incident is the one they care about. Return a reversed copy
		// rather than mutating the source.
		out := make([]btransport.SlowVRpcEvent, len(s))
		for i := range s {
			out[i] = s[len(s)-1-i]
		}
		return out
	}
	// peerShort in sessionz's slow-vRPC log historically rendered "—"
	// when the peer info was missing. The shared helper returns "" for
	// that case; the template call site handles the placeholder inline
	// via `{{else}}—{{end}}` so the visible behavior stays identical.
	m["peerShort"] = peerShort
	m["eventKindClass"] = func(k string) string {
		switch k {
		case "close":
			return "evt-close"
		case "hb-missed":
			return "evt-missed"
		case "ctx-done":
			return "evt-ctxdone"
		case "hb-alive":
			return "evt-alive"
		case "retry":
			return "evt-retry"
		}
		return ""
	}
	m["latencyClass"] = func(d time.Duration) string {
		switch {
		case d >= 5*time.Second:
			return "lat-red"
		case d >= 2*time.Second:
			return "lat-orange"
		case d >= time.Second:
			return "lat-amber"
		}
		return ""
	}
	m["clusterList"] = func(mm map[string]int64) template.HTML {
		if len(mm) == 0 {
			return template.HTML("—")
		}
		type kv struct {
			k string
			v int64
		}
		pairs := make([]kv, 0, len(mm))
		for k, v := range mm {
			pairs = append(pairs, kv{k, v})
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].v != pairs[j].v {
				return pairs[i].v > pairs[j].v
			}
			return pairs[i].k < pairs[j].k
		})
		var b strings.Builder
		b.WriteString("[")
		for i, p := range pairs {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(`<span class="mono">`)
			b.WriteString(template.HTMLEscapeString(p.k))
			b.WriteString(`</span>: `)
			b.WriteString(strconv.FormatInt(p.v, 10))
		}
		b.WriteString("]")
		return template.HTML(b.String())
	}
	m["opaqueID"] = func(n int64) string {
		if n == 0 {
			return "—"
		}
		return fmt.Sprintf("0x%016x", uint64(n))
	}
	m["scalingOutcome"] = func(ev btransport.ScalingEvent) string {
		switch {
		case ev.Requested > 0 && ev.Launched > 0:
			return strconv.Itoa(ev.Launched) + " launched"
		case ev.Requested > 0:
			return "0 launched (failed)"
		case ev.Requested < 0 && ev.Launched < 0:
			return strconv.Itoa(-ev.Launched) + " pruned"
		case ev.Requested < 0:
			return "0 pruned (none eligible)"
		default:
			return "—"
		}
	}
	m["stateChips"] = func(mm map[string]int) template.HTML {
		if len(mm) == 0 {
			return template.HTML("—")
		}
		order := []string{"New", "Starting", "Ready", "Closing", "WaitServerClose", "Closed"}
		var b strings.Builder
		for _, k := range order {
			v, ok := mm[k]
			if !ok || v == 0 {
				continue
			}
			cls := "chip"
			switch k {
			case "Ready":
				cls += " chip-active"
			case "Starting", "New":
				cls += " chip-starting"
			case "Closing", "WaitServerClose":
				cls += " chip-closing"
			case "Closed":
				cls += " chip-closed"
			}
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(`<span class="`)
			b.WriteString(cls)
			b.WriteString(`">`)
			b.WriteString(strconv.Itoa(v))
			b.WriteString(`&nbsp;`)
			b.WriteString(template.HTMLEscapeString(k))
			b.WriteString(`</span>`)
		}
		return template.HTML(b.String())
	}
	m["bucketMax"] = func(b []btransport.LifetimeBucketCount) int {
		mx := 0
		for _, x := range b {
			if x.Count > mx {
				mx = x.Count
			}
		}
		return mx
	}
	m["barWidth"] = func(count, max int) int {
		if max <= 0 {
			return 0
		}
		w := count * 100 / max
		if w == 0 && count > 0 {
			return 2
		}
		return w
	}
	m["actualRatio"] = func(sess, classic int64) string {
		total := sess + classic
		if total == 0 {
			return "—"
		}
		return strconv.FormatFloat(float64(sess)/float64(total), 'f', 2, 64)
	}
	m["sumMap"] = func(mm map[string]int64) int64 {
		var total int64
		for _, v := range mm {
			total += v
		}
		return total
	}
	m["closeReasonsShort"] = func(mm map[string]int64) string {
		if len(mm) == 0 {
			return "—"
		}
		var total int64
		for _, v := range mm {
			total += v
		}
		return strconv.FormatInt(total, 10) + " total"
	}
	m["msgBreakdown"] = func(mm map[string]int64) string {
		if len(mm) == 0 {
			return ""
		}
		keys := make([]string, 0, len(mm))
		for k := range mm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for i, k := range keys {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(strconv.FormatInt(mm[k], 10))
		}
		return b.String()
	}
	m["sparkline"] = func(width, height int, color string, values []float64) template.HTML {
		if len(values) < 2 {
			return ""
		}
		mn, mx := values[0], values[0]
		for _, v := range values {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		span := mx - mn
		if span == 0 {
			span = 1
		}
		var b strings.Builder
		b.WriteString(`<svg width="`)
		b.WriteString(strconv.Itoa(width))
		b.WriteString(`" height="`)
		b.WriteString(strconv.Itoa(height))
		b.WriteString(`" style="vertical-align:middle"><polyline fill="none" stroke="`)
		b.WriteString(color)
		b.WriteString(`" stroke-width="1.2" points="`)
		stepX := float64(width-2) / float64(len(values)-1)
		for i, v := range values {
			if i > 0 {
				b.WriteString(" ")
			}
			x := 1 + float64(i)*stepX
			y := float64(height-1) - (v-mn)/span*float64(height-2)
			b.WriteString(strconv.FormatFloat(x, 'f', 1, 64))
			b.WriteString(",")
			b.WriteString(strconv.FormatFloat(y, 'f', 1, 64))
		}
		b.WriteString(`"/></svg>`)
		return template.HTML(b.String())
	}
	m["okSeries"] = func(ts []btransport.TimeSeriesSample) []float64 {
		out := make([]float64, len(ts))
		for i, s := range ts {
			out[i] = s.OkPerSec
		}
		return out
	}
	m["errSeries"] = func(ts []btransport.TimeSeriesSample) []float64 {
		out := make([]float64, len(ts))
		for i, s := range ts {
			out[i] = s.ErrPerSec
		}
		return out
	}
	m["sessionsSeries"] = func(ts []btransport.TimeSeriesSample) []float64 {
		out := make([]float64, len(ts))
		for i, s := range ts {
			out[i] = float64(s.Sessions)
		}
		return out
	}
	m["msgCell"] = func(total int64, mm map[string]int64) template.HTML {
		if len(mm) == 0 {
			return template.HTML(strconv.FormatInt(total, 10))
		}
		keys := make([]string, 0, len(mm))
		for k := range mm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(`<details class="msgcell"><summary>`)
		b.WriteString(strconv.FormatInt(total, 10))
		b.WriteString(`</summary><div class="msgcell-body">`)
		for _, k := range keys {
			b.WriteString(`<div><span class="msgcell-k">`)
			b.WriteString(template.HTMLEscapeString(k))
			b.WriteString(`</span><span class="msgcell-v">`)
			b.WriteString(strconv.FormatInt(mm[k], 10))
			b.WriteString(`</span></div>`)
		}
		b.WriteString(`</div></details>`)
		return template.HTML(b.String())
	}
	return m
}

const sessionzIndexTplSrc = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>bigtable sessionz</title>
<meta http-equiv="refresh" content="5">
<style>
body{font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;margin:1.5em;color:#222;background:#fafafa}
h1{font-size:1.3em;margin:0 0 .5em 0}
h2{font-size:1em;color:#666;font-weight:normal;margin:0 0 1em 0}
table{border-collapse:collapse;width:100%;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.06)}
th,td{padding:.45em .8em;text-align:left;border-bottom:1px solid #eee;font-size:.92em}
th{background:#f3f3f3;font-weight:600}
tr:hover td{background:#fafafa}
.num{text-align:right;font-variant-numeric:tabular-nums}
a{color:#1a5fb4;text-decoration:none}
a:hover{text-decoration:underline}
.empty{color:#888;font-style:italic;padding:.8em 0}
.chip{display:inline-block;padding:.05em .45em;border-radius:3px;font-size:.78em;background:#eee;color:#444;font-variant-numeric:tabular-nums;white-space:nowrap}
.chip-active{background:#dff5d8;color:#197a1f}
.chip-starting{background:#fff1c8;color:#a07000}
.chip-closing{background:#ffe2cd;color:#a04500}
.chip-closed{background:#e0e0e0;color:#666}
.foot{margin-top:1.5em;color:#888;font-size:.8em}
</style>
</head><body>
<h1>Bigtable Session Pools</h1>
<h2>generated {{timestamp .Generated}} · auto-refresh 5s · <a href="../afez/">afez ▸ per-AFE view</a> · <a href="../loadz/">loadz ▸ picker decisions</a></h2>
{{if .HasProvider}}
<div style="margin-bottom:1em;background:#fff;padding:.6em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)">
<b>Diverter</b>
target session load <b>{{printf "%.2f" .Diverter.SessionLoad}}</b> ·
session picks {{.Diverter.SessionPicks}} · classic picks {{.Diverter.ClassicPicks}}
{{if or .Diverter.SessionPicks .Diverter.ClassicPicks}}
· actual ratio <b>{{actualRatio .Diverter.SessionPicks .Diverter.ClassicPicks}}</b>
{{end}}
</div>
{{end}}
{{if not .HasProvider}}
<div class="empty">Session pooling is disabled on this client (ClientConfig.EnableSessionPool is false).</div>
{{else if not .Pools}}
<div class="empty">No session pools — no session-routed traffic has run yet.</div>
{{else}}
<table>
<thead><tr>
<th>Pool</th><th>Type</th><th>Picker</th>
<th class="num">Sessions</th><th>States</th>
<th class="num">In&nbsp;use</th><th class="num">Pending</th><th class="num">Min/Max</th>
</tr></thead>
<tbody>
{{range .Pools}}
<tr>
<td><a href="pool/{{.Name}}">{{.Name}}</a></td>
<td>{{.SessionType}}</td>
<td>{{.PickerType}}</td>
<td class="num">{{.TotalSessions}}</td>
<td>{{stateChips .StateCounts}}</td>
<td class="num">{{.InUseCount}}</td>
<td class="num">{{.PendingCount}}</td>
<td class="num">{{.MinSessions}} / {{.MaxSessions}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}
<div class="foot"><a href="?format=json">JSON</a></div>
</body></html>
`

const sessionzPoolTplSrc = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>bigtable sessionz · {{.Pool.Name}}</title>
<meta http-equiv="refresh" content="5">
<style>
body{font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;margin:1.5em;color:#222;background:#fafafa}
h1{font-size:1.3em;margin:0 0 .25em 0}
h2{font-size:1em;color:#666;font-weight:normal;margin:0 0 1em 0}
table{border-collapse:collapse;width:100%;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.06)}
th,td{padding:.4em .7em;text-align:left;border-bottom:1px solid #eee;font-size:.88em;vertical-align:top}
th{background:#f3f3f3;font-weight:600}
tr:hover td{background:#fafafa}
.num{text-align:right;font-variant-numeric:tabular-nums}
.mono{font-family:ui-monospace,Consolas,monospace;font-size:.85em}
.state-active{color:#197a1f;font-weight:600}
.state-starting{color:#a07000;font-weight:600}
.state-closing{color:#a04500;font-weight:600}
.state-closed{color:#888}
tr:target td{background:#fff4c2}
tr:target td:first-child{border-left:3px solid #f0a000}
.lat-amber{background:#fff7d6;color:#7a5a00;font-weight:600}
.lat-orange{background:#ffe0c2;color:#a04500;font-weight:600}
.lat-red{background:#ffd0d0;color:#922;font-weight:700}
.evt-close{background:#ffd0d0;color:#922;font-weight:700}
.evt-missed{background:#ffe0c2;color:#a04500;font-weight:600}
.evt-ctxdone{background:#fff7d6;color:#7a5a00;font-weight:600}
.evt-alive{background:#eaf3ff;color:#1a5fb4;font-weight:600}
.evt-retry{background:#f3e6ff;color:#5a1a8a;font-weight:600}
a{color:#1a5fb4;text-decoration:none}
a:hover{text-decoration:underline}
.summary{margin-bottom:1em;background:#fff;padding:.75em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
.summary span{display:inline-block;margin-right:1.4em}
.summary b{color:#444}
.empty{color:#888;font-style:italic;padding:.8em 0}
.chip{display:inline-block;padding:.05em .45em;border-radius:3px;font-size:.78em;background:#eee;color:#444;font-variant-numeric:tabular-nums;white-space:nowrap}
.chip-active{background:#dff5d8;color:#197a1f}
.chip-starting{background:#fff1c8;color:#a07000}
.chip-closing{background:#ffe2cd;color:#a04500}
.chip-closed{background:#e0e0e0;color:#666}
.foot{margin-top:1.5em;color:#888;font-size:.8em}
details.openreq{margin-bottom:1em;background:#fff;padding:.5em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
details.openreq>summary{cursor:pointer;color:#1a5fb4;padding:.25em 0}
details.openreq>summary:hover{color:#15498a}
.openreq-body h4{font-size:.9em;margin:.8em 0 .25em 0;color:#444}
.openreq-body pre{background:#f7f7f7;padding:.6em .8em;border-left:3px solid #1a5fb4;font-family:ui-monospace,Consolas,monospace;font-size:.82em;line-height:1.4;margin:0;overflow-x:auto}
details.msgcell{display:inline-block}
details.msgcell>summary{cursor:pointer;list-style:none;color:#1a5fb4;text-decoration:underline dotted}
details.msgcell>summary::-webkit-details-marker{display:none}
details.msgcell>summary:hover{color:#15498a}
.msgcell-body{position:absolute;background:#fff;border:1px solid #ddd;box-shadow:0 4px 10px rgba(0,0,0,.12);padding:.5em .75em;margin-top:.25em;font-size:.85em;text-align:left;z-index:10;min-width:14em}
.msgcell-body div{display:flex;justify-content:space-between;gap:1em;padding:.1em 0}
.msgcell-k{color:#444;font-family:ui-monospace,Consolas,monospace}
.msgcell-v{font-variant-numeric:tabular-nums;color:#222}
</style>
</head><body>
<h1>Pool <span class="mono">{{.Pool.Name}}</span></h1>
<h2><a href="../">← all pools</a> · generated {{timestamp .Generated}} · auto-refresh 5s</h2>
<div class="summary">
<span><b>Type</b> {{.Pool.SessionType}}</span>
<span><b>Picker</b> {{.Pool.PickerType}}</span>
<span><b>Min / Max</b> {{.Pool.MinSessions}} / {{.Pool.MaxSessions}}</span>
<span><b>Total</b> {{.Pool.TotalSessions}}</span>
<span><b>States</b> {{stateChips .Pool.StateCounts}}</span>
<span><b>In&nbsp;use</b> {{.Pool.InUseCount}}</span>
<span><b>Pending</b> {{.Pool.PendingCount}}</span>
<span><b>Starting</b> {{.Pool.StartingCount}}</span>
</div>
<div class="summary">
<span><b>Sessions opened</b> {{.Pool.SessionsOpened}}</span>
<span><b>Sessions closed</b> {{.Pool.SessionsClosed}}</span>
<span><b>Close reasons</b> {{msgCell (sumMap .Pool.CloseReasons) .Pool.CloseReasons}}</span>
<span><b>Config listener fires</b> {{.Pool.ListenerFires}}</span>
<span><b>Creation budget</b> {{.Pool.Throttler.InUse}} / {{.Pool.Throttler.Capacity}} (penalty {{dur .Pool.Throttler.PenaltyDuration}})</span>
</div>
<div class="summary">
<span title="end-to-end wall-clock observed by SessionPoolImpl.Invoke — includes pool checkout wait + network + decode + Backend"><b>TotalLatency</b> p50 {{dur .Pool.TotalLatencyP50}} · p95 {{dur .Pool.TotalLatencyP95}} · p99 {{dur .Pool.TotalLatencyP99}} <span class="mono">(n={{.Pool.TotalLatencyN}})</span></span>
<span title="java-parity ClientTransportLatency = (stream Send→Recv) − server-reported Backend; wire + AFE + client-decode overhead outside server processing"><b>TransportLatency</b> p50 {{dur .Pool.TransportLatencyP50}} · p95 {{dur .Pool.TransportLatencyP95}} · p99 {{dur .Pool.TransportLatencyP99}} <span class="mono">(n={{.Pool.TransportLatencyN}})</span></span>
<span title="server-reported SessionRequestStats.BackendLatency — pure server processing time"><b>BackendLatency</b> p50 {{dur .Pool.LatencyP50}} · p95 {{dur .Pool.LatencyP95}} · p99 {{dur .Pool.LatencyP99}} <span class="mono">(n={{.Pool.LatencyN}})</span></span>
{{if .Pool.TimeSeries}}
<span><b>sessions</b> {{sparkline 120 28 "#1a5fb4" (sessionsSeries .Pool.TimeSeries)}}</span>
<span><b>ok/s</b> {{sparkline 120 28 "#197a1f" (okSeries .Pool.TimeSeries)}}</span>
<span><b>err/s</b> {{sparkline 120 28 "#a04500" (errSeries .Pool.TimeSeries)}}</span>
{{end}}
</div>

{{if .Pool.ClusterCounts}}
<div class="summary">
<b>Clusters</b> ({{sumMap .Pool.ClusterCounts}} total responses) — {{clusterList .Pool.ClusterCounts}}
</div>
{{end}}

{{if .Pool.OpenRequest}}
<details class="openreq">
<summary><b>OpenSessionRequest</b> <span class="mono">{{.Pool.OpenRequest.PayloadType}}</span> (protocol v{{.Pool.OpenRequest.ProtocolVersion}}) — click to expand</summary>
<div class="openreq-body">
{{if .Pool.OpenRequest.PayloadJSON}}
<h4>Payload</h4>
<pre>{{.Pool.OpenRequest.PayloadJSON}}</pre>
{{end}}
{{if .Pool.OpenRequest.FlagsJSON}}
<h4>FeatureFlags</h4>
<pre>{{.Pool.OpenRequest.FlagsJSON}}</pre>
{{end}}
</div>
</details>
{{end}}
{{if not .Pool.Sessions}}
<div class="empty">No sessions registered in this pool right now.</div>
{{else}}
<table>
<thead><tr>
<th>Session</th><th>State</th><th>Transport</th><th>AFE region</th><th>AFE subzone</th>
<th class="num">GFE&nbsp;id</th><th class="num">AFE&nbsp;id</th>
<th class="num">Ch&nbsp;#</th>
<th class="num">OK</th><th class="num">Err</th><th class="num">Retries</th>
<th class="num">Msgs&nbsp;sent</th><th class="num">Msgs&nbsp;recv</th><th class="num">In&nbsp;flight</th>
<th class="num">Outstanding</th><th class="num">Picks</th>
<th class="num">p50</th><th class="num">p95</th><th class="num">p99</th>
<th>Last&nbsp;activity</th><th>Last&nbsp;state&nbsp;change</th><th>Next&nbsp;heartbeat</th>
</tr></thead>
<tbody>
{{range .Pool.Sessions}}
<tr id="{{.LogName}}">
<td class="mono">{{.LogName}}</td>
<td class="{{stateClass .State}}">{{.State}}</td>
<td>{{orDash .Peer.TransportType}}</td>
<td>{{orDash .Peer.ApplicationFrontendRegion}}</td>
<td>{{orDash .Peer.ApplicationFrontendSubzone}}</td>
<td class="mono num" title="opaque int64; rendered as hex of the uint64 bit pattern">{{opaqueID .Peer.GoogleFrontendID}}</td>
<td class="mono num" title="opaque int64; rendered as hex of the uint64 bit pattern">{{opaqueID .Peer.ApplicationFrontendID}}</td>
<td class="num">{{if ge .ChannelIndex 0}}<a href="../../channelz/#channel-session-{{.ChannelIndex}}" title="jump to this channel in channelz">{{.ChannelIndex}}</a>{{else}}—{{end}}</td>
<td class="num">{{.OkRpcs}}</td>
<td class="num">{{.ErrorRpcs}}</td>
<td class="num">{{.Retries}}</td>
<td class="num">{{msgCell .MsgsSent .MsgsSentByType}}</td>
<td class="num">{{msgCell .MsgsRecv .MsgsRecvByType}}</td>
<td class="num">{{.ActiveRpcs}}</td>
<td class="num">{{.Handle.Outstanding}}</td>
<td class="num">{{.Handle.Picks}}</td>
<td class="num">{{dur .LatencyP50}}</td>
<td class="num">{{dur .LatencyP95}}</td>
<td class="num">{{dur .LatencyP99}}</td>
<td>{{age .Handle.LastActivity}} ago</td>
<td>{{age .LastStateChange}} ago</td>
<td>{{untilNow .NextHeartbeat}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}

{{if .Pool.LifetimeHistogram}}
<h3 style="font-size:1em;margin:1.4em 0 .4em 0;color:#444">Session lifetimes (n={{.Pool.LifetimeN}}) · p50 {{dur .Pool.LifetimeP50}} · p95 {{dur .Pool.LifetimeP95}} · p99 {{dur .Pool.LifetimeP99}}</h3>
<table>
<thead><tr><th>Bucket</th><th class="num">Count</th><th>Distribution</th></tr></thead>
<tbody>
{{$max := bucketMax .Pool.LifetimeHistogram}}
{{range .Pool.LifetimeHistogram}}
<tr>
<td class="mono">{{.Label}}</td>
<td class="num">{{.Count}}</td>
<td><div style="display:inline-block;background:#1a5fb4;height:.9em;width:{{barWidth .Count $max}}%;min-width:1px"></div></td>
</tr>
{{end}}
</tbody>
</table>
<div style="font-size:.78em;color:#888;margin-top:.3em">
Captures the lifetime (admission → retirement) of the most recent {{.Pool.LifetimeN}} closed sessions.
A spike in the &lt;1m bucket indicates churn (sessions dying young — usually GoAway / Heartbeat / Error).
</div>
{{end}}

{{if .Pool.RecentEvents}}
<h3 style="font-size:1em;margin:1.4em 0 .4em 0;color:#444">Recent session events (last {{len .Pool.RecentEvents}}, newest first)</h3>
<table>
<thead><tr>
<th>When</th><th>Kind</th><th>Session</th><th>Detail</th>
</tr></thead>
<tbody>
{{range .Pool.RecentEvents}}
<tr>
<td>{{age .At}} ago</td>
<td class="mono {{eventKindClass .Kind}}">{{.Kind}}</td>
<td class="mono">{{.Session}}</td>
<td class="mono" style="white-space:pre-wrap;word-break:break-all">{{.Message}}</td>
</tr>
{{end}}
</tbody>
</table>
<div style="font-size:.78em;color:#888;margin-top:.3em">
<b>close</b> — stream tear-down handled by readLoop's Recv error path (raw_err shows the gRPC status).
<b>hb-missed</b> — heartbeat watchdog fired ForceClose; "case 2" half-dead Recv.
<b>hb-alive</b> — timer tick saw in-flight RPCs AND a recent frame had already pushed the deadline (≥1 interval); "case 1" server kept stream alive but specific vRPC response may be stalled.
<b>ctx-done</b> — Session.Invoke's per-attempt wait was killed by caller ctx (deadline or cancel); often paired with a 5s row in the slow-vRPC table.
<b>retry</b> — this session received attempt N (N>1); detail carries the previous attempt's gRPC code + message that triggered the retry (paired with the per-session Retries counter).
</div>
{{end}}

{{if .Pool.SlowVRpcs}}
<h3 style="font-size:1em;margin:1.4em 0 .4em 0;color:#444">Slow vRPCs (last {{len .Pool.SlowVRpcs}}, newest first)</h3>
<table>
<thead><tr>
<th>When</th><th>Method</th><th class="num">Latency</th>
<th class="num">PoolWait</th><th class="num">Transport</th><th class="num">Backend</th>
<th>Session</th><th class="num">RpcID</th><th class="num">SessionAge</th>
<th>Peer (afe/region/subzone)</th>
<th>Remote (→ tcpz)</th>
<th>Status</th>
</tr></thead>
<tbody>
{{range reverseSlow .Pool.SlowVRpcs}}
<tr>
<td>{{age .At}} ago</td>
<td class="mono">{{.Method}}</td>
<td class="num {{latencyClass .Latency}}">{{dur .Latency}}</td>
<td class="num" title="time inside CheckoutSession — waiting for an idle session at the pool boundary">{{dur .PoolWait}}</td>
<td class="num" title="java-parity ClientTransportLatency = (stream Send→Recv) − Backend; wire + AFE + client-decode overhead outside server processing">{{dur .TransportLatency}}</td>
<td class="num" title="server-reported BackendLatency">{{dur .BackendLatency}}</td>
<td class="mono">{{.Session}}</td>
<td class="num" title="per-session 1-indexed RPC id; small values indicate a fresh session">{{.RPCIDOnSession}}</td>
<td class="num" title="age of the session at the time of this call">{{dur .SessionAge}}</td>
<td class="mono" title="ApplicationFrontendId (hex) / region / subzone of the AFE this session was bound to at call time">{{with (peerShort .Peer)}}{{.}}{{else}}—{{end}}</td>
<td class="mono" title="TCP remote (AFE) addr this session's stream is bound to. Click to filter tcpz to the conn(s) with this remote.">{{if .RemoteAddr}}<a href="../../tcpz/?remote={{.RemoteAddr | urlquery}}">{{.RemoteAddr}}</a>{{else}}—{{end}}</td>
<td>{{if .Success}}OK{{else}}<span style="color:#922">{{.ErrCode}}</span>{{end}}</td>
</tr>
{{end}}
</tbody>
</table>
<div style="font-size:.78em;color:#888;margin-top:.3em">
<b>PoolWait</b> ≈ <b>Latency</b> → workers exceeded pool capacity; queued at the pool boundary waiting for an idle session.
<b>Transport</b> ≫ <b>Backend</b> → time was spent on the wire (network RTT / server queue), not in server processing.
<b>Backend</b> close to <b>Latency</b> → server itself was slow.
Low <b>RpcID</b> + small <b>SessionAge</b> → fresh session warm-up cost.
<b>Remote</b> → click to filter tcpz to the conn this session is bound to (per-conn TCP_INFO: RTT/cwnd/retrans).
</div>
{{end}}

{{if .Pool.ScalingHistory}}
<h3 style="font-size:1em;margin:1.4em 0 .4em 0;color:#444">Scaling history (newest last, last {{len .Pool.ScalingHistory}})</h3>
<table>
<thead><tr>
<th>When</th>
<th class="num">Pool&nbsp;was</th>
<th class="num">Sizer&nbsp;asked</th>
<th class="num">Action&nbsp;result</th>
<th>Branch</th>
<th class="num" title="Stats at decision time: ready/starting/in-use/pending(waiters)">R/S/U/P</th>
<th class="num" title="Sizer intermediates: effective-pending / sessions-in-use / idle-cushion / desired-capacity">eP/sU/idle/desired</th>
<th class="num" title="Immediate vs eventual capacity (ready vs ready+starting)">imm/evt</th>
<th>Reason</th>
</tr></thead>
<tbody>
{{range .Pool.ScalingHistory}}
<tr>
<td>{{age .At}} ago</td>
<td class="num">{{.Before}}</td>
<td class="num">{{signed .Requested}}</td>
<td class="num">{{scalingOutcome .}}</td>
<td class="mono" title="scale-up | scale-down (advisory — pool shrinks via non-replacement in OnClose, not via prune) | dead-band | no-stats">{{.Decision.Branch}}</td>
<td class="num mono" title="ready={{.Decision.ReadyCount}}&#10;starting={{.Decision.StartingCount}}&#10;in_use={{.Decision.InUseCount}}&#10;pending(waiters)={{.Decision.PendingCount}}">{{.Decision.ReadyCount}}/{{.Decision.StartingCount}}/{{.Decision.InUseCount}}/{{.Decision.PendingCount}}</td>
<td class="num mono" title="effectivePending = ceil(pending/{{.Decision.NewSessionQLen}}) = {{.Decision.EffectivePending}}&#10;sessionsInUse = inUse + effPending = {{.Decision.SessionsInUse}}&#10;idle = max(minIdle={{.Decision.MinIdleSessions}}, ceil(sessionsInUse*{{.Decision.HeadroomPct}})) = {{.Decision.IdleHeadroom}}&#10;desired = clamp(sessionsInUse+idle, min={{.Decision.MinSessions}}, max={{.Decision.MaxSessions}}) = {{.Decision.DesiredCapacity}}">{{.Decision.EffectivePending}}/{{.Decision.SessionsInUse}}/{{.Decision.IdleHeadroom}}/{{.Decision.DesiredCapacity}}</td>
<td class="num mono" title="immediate = ready ({{.Decision.ImmediateCapacity}})&#10;eventual = ready + starting ({{.Decision.EventualCapacity}})&#10;Scale up if desired>eventual; scale-down is advisory (pool shrinks by non-replacement in OnClose); dead-band otherwise.">{{.Decision.ImmediateCapacity}}/{{.Decision.EventualCapacity}}</td>
<td>{{.Reason}}</td>
</tr>
{{end}}
</tbody>
</table>
<div style="font-size:.78em;color:#888;margin-top:.3em">
<b>Pool&nbsp;was</b> = live pool size when the sizer decided.
<b>Sizer&nbsp;asked</b> = delta requested (+ scale up, − advisory scale-down; scale-down deltas are informational — the pool shrinks passively via OnClose's replace-on-death gate, mirroring java-bigtable).
<b>Action&nbsp;result</b> = sessions launched (scale up — these are handshaking and become Active shortly after).
<b>R/S/U/P</b> = Ready / Starting / In-use / Pending(waiters) at decision time.
<b>eP/sU/idle/desired</b> = effectivePending / sessionsInUse / idleCushion / desiredCapacity — hover for the formula.
<b>imm/evt</b> = immediate (ready) vs eventual (ready+starting) capacity; the dead band between them prevents pruning sessions whose handshake is still landing.
</div>
{{end}}

<div class="foot"><a href="?format=json">JSON</a> · <a href="../">all pools</a></div>
</body></html>
`

var (
	sessionzIndexTpl = template.Must(template.New("sessionz-index").Funcs(sessionzFuncs()).Parse(sessionzIndexTplSrc))
	sessionzPoolTpl  = template.Must(template.New("sessionz-pool").Funcs(sessionzFuncs()).Parse(sessionzPoolTplSrc))
)
