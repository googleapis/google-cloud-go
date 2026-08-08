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

// debugtagsz view — every "shouldn't happen" tag emitted by the session
// pool / session / configuration manager since process start, with
// per-tag counts and first/last-seen timestamps. Backed by the
// process-global tracer in bigtable/internal/transport; independent of
// any specific Client, so this page renders even when no Client has
// been created yet (empty state).

package debugview

import (
	"html/template"
	"net/http"
	"time"

	btransport "cloud.google.com/go/bigtable/internal/transport"
)

func newDebugtagszHandler() http.Handler {
	mux := http.NewServeMux()
	srv := &debugtagszServer{}
	mux.HandleFunc("/", srv.handle)
	return mux
}

type debugtagszServer struct{}

// debugtagszRow is one row of the rendered table. Wraps the raw
// snapshot with a Recent flag so the template can highlight tags that
// fired within the last minute — the "something just went wrong"
// signal an on-call would care about.
type debugtagszRow struct {
	Name      string
	Count     int64
	FirstSeen time.Time
	LastSeen  time.Time
	Recent    bool // LastSeen within the last minute
}

type debugtagszPageData struct {
	Rows       []debugtagszRow
	TotalTags  int
	TotalCount int64
	MostRecent time.Time
	Generated  time.Time
}

func (s *debugtagszServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	// Raw slice, already sorted by LastSeen descending inside the tracer.
	snaps := btransport.DebugTags()

	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, snaps)
		return
	}

	now := time.Now()
	rows := make([]debugtagszRow, 0, len(snaps))
	var totalCount int64
	var mostRecent time.Time
	for _, snap := range snaps {
		totalCount += snap.Count
		if snap.LastSeen.After(mostRecent) {
			mostRecent = snap.LastSeen
		}
		rows = append(rows, debugtagszRow{
			Name:      snap.Name,
			Count:     snap.Count,
			FirstSeen: snap.FirstSeen,
			LastSeen:  snap.LastSeen,
			Recent:    now.Sub(snap.LastSeen) < time.Minute,
		})
	}

	writeHTML(w, debugtagszTpl, debugtagszPageData{
		Rows:       rows,
		TotalTags:  len(rows),
		TotalCount: totalCount,
		MostRecent: mostRecent,
		Generated:  now,
	})
}

func debugtagszFuncs() template.FuncMap {
	m := commonFuncs()
	m["ago"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return roundDurationLong(time.Since(t)).String() + " ago"
	}
	return m
}

var debugtagszTpl = template.Must(template.New("debugtagsz").Funcs(debugtagszFuncs()).Parse(debugtagszTplSrc))

const debugtagszTplSrc = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>bigtable debugtagsz{{if .TotalTags}} — {{.TotalTags}} tags · {{.TotalCount}} events{{end}}</title>
<meta http-equiv="refresh" content="5">
<style>
body{font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;margin:1.5em;color:#222;background:#fafafa}
h1{font-size:1.3em;margin:0 0 .25em 0}
h2{font-size:1em;color:#666;font-weight:normal;margin:0 0 1em 0}
.summary{margin-bottom:1em;background:#fff;padding:.75em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
.summary span{display:inline-block;margin-right:1.6em}
.summary b{color:#444}
.empty{color:#888;font-style:italic;padding:.8em 0;background:#fff;padding:1.2em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
table{border-collapse:collapse;width:100%;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.06)}
th,td{padding:.4em .7em;text-align:left;border-bottom:1px solid #eee;font-size:.9em}
th{background:#f3f3f3;font-weight:600;color:#444}
td.num{text-align:right;font-variant-numeric:tabular-nums}
td.name{font-family:ui-monospace,Consolas,monospace;font-size:.88em}
tr.recent td{background:#fff5e0}
tr.recent td.name{color:#a04500;font-weight:600}
.pill{display:inline-block;padding:.05em .5em;border-radius:2px;background:#fdecea;color:#b32222;font-size:.78em;font-weight:600;margin-left:.4em}
.foot{margin-top:1.5em;color:#888;font-size:.8em}
a{color:#1a5fb4;text-decoration:none}
a:hover{text-decoration:underline}
.desc{color:#666;font-size:.85em;margin-top:.4em;max-width:52em}
</style>
</head><body>
<h1>Bigtable debug tags</h1>
<h2>generated {{timestamp .Generated}} · auto-refresh 5s</h2>

<div class="desc">
Counters for events the client code doesn't expect to reach —
wrong-state transitions, dropped GOAWAYs, orphaned vRPC responses,
watchdog-triggered closes. Each row is one time series (also emitted to
the OTel counter <span class="name">bigtable.googleapis.com/internal/client/debug_tags</span>).
Rows shaded orange fired within the last minute.
</div>

{{if not .TotalTags}}
<div class="empty">No debug tags have fired since process start. Either the session pool has been behaving cleanly, or no traffic has run through it yet.</div>
{{else}}
<div class="summary">
<span><b>Distinct tags</b> {{.TotalTags}}</span>
<span><b>Total events</b> {{.TotalCount}}</span>
<span><b>Most recent</b> {{ago .MostRecent}}</span>
</div>

<table>
<thead><tr>
<th>Tag</th>
<th style="text-align:right">Count</th>
<th>First seen</th>
<th>Last seen</th>
</tr></thead>
<tbody>
{{range .Rows}}
<tr{{if .Recent}} class="recent"{{end}}>
<td class="name">{{.Name}}{{if .Recent}}<span class="pill">just now</span>{{end}}</td>
<td class="num">{{.Count}}</td>
<td>{{ago .FirstSeen}}</td>
<td>{{ago .LastSeen}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}

<div class="foot"><a href="?format=json">JSON</a></div>
</body></html>
`
