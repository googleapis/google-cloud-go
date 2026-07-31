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

// channelz view — one row per gRPC channel with outstanding unary /
// streaming load, error count, ALTS/DirectAccess flag, IP protocol,
// gRPC connectivity state, age, and draining flag.

package debugview

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/bigtable"
)

func newChannelzHandler(p bigtable.ChannelDebugProvider) http.Handler {
	mux := http.NewServeMux()
	srv := &channelzServer{provider: p}
	mux.HandleFunc("/", srv.handle)
	return mux
}

type channelzServer struct {
	provider bigtable.ChannelDebugProvider
}

func (s *channelzServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	var pools []bigtable.ChannelPoolDebug
	if s.provider != nil {
		pools = s.provider.Snapshot()
	}

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(pools)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// If Execute errors partway through, the body has already been
	// (partially) written and the status code is committed to 200 —
	// calling http.Error here would trigger a "superfluous
	// WriteHeader" warning without doing anything useful for the
	// client. Silently swallow; the partial page is what the caller
	// gets to see.
	_ = channelzTpl.Execute(w, channelzPageData{
		Pools:       pools,
		Generated:   time.Now(),
		HasProvider: s.provider != nil,
	})
}

type channelzPageData struct {
	Pools       []bigtable.ChannelPoolDebug
	Generated   time.Time
	HasProvider bool
}

func channelzFuncs() template.FuncMap {
	m := commonFuncs()
	m["age"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return roundDurationLong(time.Since(t)).String() + " ago"
	}
	m["until"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		d := time.Until(t)
		if d < 0 {
			return "expired " + roundDurationLong(-d).String() + " ago"
		}
		return "in " + roundDurationLong(d).String()
	}
	// sessionsOn renders inline links from a channel row into the matching
	// sessionz pool-detail anchor. Uses a relative "../sessionz/..." path
	// which resolves correctly whether debugview is mounted at "/debug/" or
	// any other prefix (channelz and sessionz are always siblings).
	m["sessionsOn"] = func(byIdx map[int][]bigtable.SessionRef, idx int) template.HTML {
		if byIdx == nil {
			return template.HTML("—")
		}
		refs, ok := byIdx[idx]
		if !ok || len(refs) == 0 {
			return template.HTML("—")
		}
		var b strings.Builder
		for i, r := range refs {
			if i > 0 {
				b.WriteString(", ")
			}
			poolEsc := template.HTMLEscapeString(r.PoolName)
			nameEsc := template.HTMLEscapeString(r.LogName)
			b.WriteString(`<a href="../sessionz/pool/`)
			b.WriteString(poolEsc)
			b.WriteString(`#`)
			b.WriteString(nameEsc)
			b.WriteString(`" title="jump to `)
			b.WriteString(nameEsc)
			b.WriteString(` in `)
			b.WriteString(poolEsc)
			b.WriteString(`">`)
			b.WriteString(nameEsc)
			b.WriteString(`</a>`)
		}
		return template.HTML(b.String())
	}
	m["connStateClass"] = func(s string) string {
		switch s {
		case "READY":
			return "state-active"
		case "CONNECTING":
			return "state-starting"
		case "TRANSIENT_FAILURE":
			return "state-closing"
		case "SHUTDOWN":
			return "state-closed"
		}
		return ""
	}
	return m
}

var channelzTpl = template.Must(template.New("channelz").Funcs(channelzFuncs()).Parse(channelzTplSrc))

const channelzTplSrc = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>bigtable channelz</title>
<meta http-equiv="refresh" content="5">
<style>
body{font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;margin:1.5em;color:#222;background:#fafafa}
h1{font-size:1.3em;margin:0 0 .25em 0}
h2{font-size:1em;color:#666;font-weight:normal;margin:.6em 0 .8em 0}
h3{font-size:1.05em;margin:1.4em 0 .5em 0}
.summary{margin-bottom:1em;background:#fff;padding:.6em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
.summary span{display:inline-block;margin-right:1.4em}
.summary b{color:#444}
table{border-collapse:collapse;width:100%;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.06);margin-bottom:1em}
th,td{padding:.4em .7em;text-align:left;border-bottom:1px solid #eee;font-size:.88em;vertical-align:top}
th{background:#f3f3f3;font-weight:600}
tr:hover td{background:#fafafa}
.num{text-align:right;font-variant-numeric:tabular-nums}
.mono{font-family:ui-monospace,Consolas,monospace;font-size:.85em}
.empty{color:#888;font-style:italic;padding:.8em 0}
.state-active{color:#197a1f;font-weight:600}
.state-starting{color:#a07000;font-weight:600}
.state-closing{color:#a04500;font-weight:600}
.state-closed{color:#888}
tr:target td{background:#fff4c2}
tr:target td:first-child{border-left:3px solid #f0a000}
.foot{margin-top:1.5em;color:#888;font-size:.8em}
a{color:#1a5fb4;text-decoration:none}
a:hover{text-decoration:underline}
</style>
</head><body>
<h1>Bigtable gRPC Channel Pools</h1>
<h2>generated {{timestamp .Generated}} · auto-refresh 5s</h2>

{{if not .HasProvider}}
<div class="empty">No channel debug provider attached.</div>
{{else if not .Pools}}
<div class="empty">No Bigtable channel pools — either the client uses an externally-supplied gRPC connection (option.WithGRPCConn) or no traffic has run yet.</div>
{{else}}
{{range .Pools}}
{{$role := .Role}}
{{$sessionsByChan := .SessionsByChannel}}
<h3 id="pool-{{.Role}}">{{.Role}} pool</h3>
<div class="summary">
<span><b>Instance</b> <span class="mono">{{orDash .InstanceName}}</span></span>
<span><b>App&nbsp;profile</b> <span class="mono">{{orDash .AppProfile}}</span></span>
<span><b>LB&nbsp;policy</b> {{orDash .Snapshot.LBPolicy}}</span>
<span><b>Channels</b> {{.Snapshot.TotalConns}}</span>
</div>
{{if not .Snapshot.Channels}}
<div class="empty">No live channels.</div>
{{else}}
<table>
<thead><tr>
<th class="num">#</th><th>gRPC state</th><th>ALTS</th><th>IP</th><th>Draining</th>
<th class="num">Unary&nbsp;in&nbsp;flight</th><th class="num">Streaming&nbsp;in&nbsp;flight</th><th class="num">Picks</th><th class="num">Errors</th>
<th>Last&nbsp;activity</th><th>Age</th><th>Penalty</th><th>Sessions</th>
</tr></thead>
<tbody>
{{range .Snapshot.Channels}}
<tr id="channel-{{$role}}-{{.Index}}">
<td class="num">{{.Index}}</td>
<td class="{{connStateClass .TargetState}}">{{orDash .TargetState}}</td>
<td>{{boolMark .IsALTSUsed}}</td>
<td>{{orDash .IPProtocol}}</td>
<td>{{boolMark .IsDraining}}</td>
<td class="num">{{.OutstandingUnary}}</td>
<td class="num">{{.OutstandingStreaming}}</td>
<td class="num">{{.Picks}}</td>
<td class="num">{{.ErrorCount}}</td>
<td>{{age .LastActivity}}</td>
<td>{{age .CreatedAt}}</td>
<td>{{until .PenaltyExpiresAt}}</td>
<td class="mono" style="font-size:.78em;color:#444">{{sessionsOn $sessionsByChan .Index}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}
{{end}}
{{end}}

<div class="foot"><a href="?format=json">JSON</a></div>
</body></html>
`
