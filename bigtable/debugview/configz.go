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

// configz view — instance / app profile, last fetched timestamp,
// validity window, last error (if any), and the server's raw
// ClientConfiguration JSON.

package debugview

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/protobuf/encoding/protojson"
)

func newConfigzHandler(p bigtable.ConfigDebugProvider) http.Handler {
	mux := http.NewServeMux()
	srv := &configzServer{provider: p}
	mux.HandleFunc("/", srv.handle)
	return mux
}

type configzServer struct {
	provider bigtable.ConfigDebugProvider
}

// configzMarshaler is shared across requests — it only carries
// formatting options.
var configzMarshaler = protojson.MarshalOptions{
	Multiline:       true,
	Indent:          "  ",
	UseProtoNames:   true,
	EmitUnpopulated: false,
}

func (s *configzServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	var snap btransport.ConfigSnapshot
	if s.provider != nil {
		snap = s.provider.Snapshot()
	}

	var jsonStr string
	if snap.Response != nil {
		b, err := configzMarshaler.Marshal(snap.Response)
		if err != nil {
			jsonStr = fmt.Sprintf("error marshaling proto: %v", err)
		} else {
			jsonStr = string(b)
		}
	}

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		if jsonStr == "" {
			_, _ = w.Write([]byte("null"))
			return
		}
		_, _ = w.Write([]byte(jsonStr))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := configzTpl.Execute(w, configzPageData{
		HasProvider: s.provider != nil,
		Snapshot:    snap,
		ConfigJSON:  jsonStr,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type configzPageData struct {
	HasProvider bool
	Snapshot    btransport.ConfigSnapshot
	ConfigJSON  string
}

func configzFuncs() template.FuncMap {
	m := commonFuncs()
	m["ago"] = func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return roundDurationLong(time.Since(t)).String() + " ago"
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
	m["errString"] = func(e error) string {
		if e == nil {
			return ""
		}
		return e.Error()
	}
	return m
}

var configzTpl = template.Must(template.New("configz").Funcs(configzFuncs()).Parse(configzTplSrc))

const configzTplSrc = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>bigtable configz</title>
<meta http-equiv="refresh" content="10">
<style>
body{font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;margin:1.5em;color:#222;background:#fafafa}
h1{font-size:1.3em;margin:0 0 .25em 0}
h2{font-size:1em;color:#666;font-weight:normal;margin:0 0 1em 0}
.summary{margin-bottom:1em;background:#fff;padding:.75em 1em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
.summary span{display:inline-block;margin-right:1.4em}
.summary b{color:#444}
.error{background:#fff5f5;border-left:4px solid #c33;padding:.75em 1em;margin-bottom:1em;color:#622;box-shadow:0 1px 2px rgba(0,0,0,.06)}
.error b{color:#922}
pre{background:#fff;padding:1em;border-left:4px solid #1a5fb4;box-shadow:0 1px 2px rgba(0,0,0,.06);overflow-x:auto;font-family:ui-monospace,Consolas,monospace;font-size:.88em;line-height:1.4;margin:0}
.empty{color:#888;font-style:italic;padding:.8em 0}
.mono{font-family:ui-monospace,Consolas,monospace;font-size:.85em}
.foot{margin-top:1.5em;color:#888;font-size:.8em}
a{color:#1a5fb4;text-decoration:none}
a:hover{text-decoration:underline}
</style>
</head><body>
<h1>Bigtable GetClientConfiguration</h1>
<h2>generated {{timestamp .Snapshot.CapturedAt}} · auto-refresh 10s</h2>

{{if not .HasProvider}}
<div class="empty">No configuration manager attached to this client.</div>
{{else}}
<div class="summary">
<span><b>Instance</b> <span class="mono">{{orDash .Snapshot.InstanceName}}</span></span>
<span><b>App&nbsp;profile</b> <span class="mono">{{orDash .Snapshot.AppProfileID}}</span></span>
<span><b>Config&nbsp;seq</b> {{.Snapshot.ConfigSeq}}</span>
<span><b>Last&nbsp;fetched</b> {{ago .Snapshot.FetchedAt}}</span>
<span><b>Valid</b> {{untilNow .Snapshot.ValidUntil}}</span>
</div>

{{if .Snapshot.LastErr}}
<div class="error">
<b>Last poll error</b> ({{ago .Snapshot.LastErrAt}}): <span class="mono">{{errString .Snapshot.LastErr}}</span>
</div>
{{end}}

{{if .ConfigJSON}}
<pre>{{.ConfigJSON}}</pre>
{{else}}
<div class="empty">No successful GetClientConfiguration poll has completed yet.</div>
{{end}}

{{if .Snapshot.PollHistory}}
<h3 style="font-size:1em;margin:1.4em 0 .4em 0;color:#444">Poll history (oldest first, last {{len .Snapshot.PollHistory}})</h3>
<table style="border-collapse:collapse;width:100%;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.06)">
<thead><tr style="background:#f3f3f3">
<th style="padding:.4em .7em;text-align:left;font-size:.88em">When</th>
<th style="padding:.4em .7em;text-align:right;font-size:.88em">Duration</th>
<th style="padding:.4em .7em;text-align:right;font-size:.88em">Seq</th>
<th style="padding:.4em .7em;text-align:left;font-size:.88em">Result</th>
</tr></thead>
<tbody>
{{range .Snapshot.PollHistory}}
<tr>
<td style="padding:.35em .7em;border-bottom:1px solid #eee;font-size:.85em">{{ago .At}}</td>
<td style="padding:.35em .7em;border-bottom:1px solid #eee;font-size:.85em;text-align:right;font-variant-numeric:tabular-nums">{{.Duration}}</td>
<td style="padding:.35em .7em;border-bottom:1px solid #eee;font-size:.85em;text-align:right;font-variant-numeric:tabular-nums">{{.ConfigSeq}}</td>
<td style="padding:.35em .7em;border-bottom:1px solid #eee;font-size:.85em">{{if .Err}}<span style="color:#922">{{.Err}}</span>{{else}}<span style="color:#197a1f">OK</span>{{end}}</td>
</tr>
{{end}}
</tbody>
</table>
{{end}}
{{end}}

<div class="foot"><a href="?format=json">JSON</a></div>
</body></html>
`
