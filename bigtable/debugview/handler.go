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

// Package debugview mounts every Bigtable -z debug page (sessionz / afez /
// loadz / channelz / configz / tcpz / debugtagsz) behind a single
// http.Handler. Serves a link index at "/" and each view at "/<view>/".
//
// Typical wiring is one line:
//
//	client, _ := bigtable.NewClientWithConfig(ctx, "p", "i",
//	    bigtable.ClientConfig{EnableDebug: true})
//	http.Handle("/debug/", http.StripPrefix("/debug",
//	    debugview.Handler(client)))
//
// Handler takes a *bigtable.Client directly — TCPStats + the three
// debug providers are pulled off the client via its accessors. Passing
// a nil *bigtable.Client is fine: every view falls back to its "not
// enabled" empty state.
package debugview

import (
	"html/template"
	"net/http"
	"time"

	"cloud.google.com/go/bigtable"
)

// Handler returns the combined debug mux. See package doc for the
// routes it exposes. Passing a nil *bigtable.Client is safe — every
// client-backed view falls back to its "not enabled" empty state.
// Panics on template parse errors would surface at package-init time
// (see the per-view *TplSrc constants), not here.
func Handler(c *bigtable.Client) http.Handler {
	mux := http.NewServeMux()

	var (
		sessionProv bigtable.SessionDebugProvider
		channelProv bigtable.ChannelDebugProvider
		configProv  bigtable.ConfigDebugProvider
		stats       *bigtable.TCPStats
	)
	if c != nil {
		sessionProv = c.SessionDebug()
		channelProv = c.ChannelDebug()
		configProv = c.ConfigDebug()
		stats = c.TCPStats()
	}

	mux.Handle("/sessionz/", http.StripPrefix("/sessionz", newSessionzHandler(sessionProv)))
	mux.Handle("/afez/", http.StripPrefix("/afez", newAfezHandler(sessionProv)))
	mux.Handle("/loadz/", http.StripPrefix("/loadz", newLoadzHandler(sessionProv)))
	mux.Handle("/channelz/", http.StripPrefix("/channelz", newChannelzHandler(channelProv)))
	mux.Handle("/configz/", http.StripPrefix("/configz", newConfigzHandler(configProv)))
	mux.Handle("/tcpz/", http.StripPrefix("/tcpz", newTcpzHandler(stats)))
	// debugtagsz reads a process-global tracer — no per-Client wiring
	// needed; mounts even when c is nil.
	mux.Handle("/debugtagsz/", http.StripPrefix("/debugtagsz", newDebugtagszHandler()))

	// Index page lives at the root. Anything else that lands here (a
	// mis-typed sub-path) 404s cleanly rather than serving the index.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		writeHTML(w, indexTpl, indexPageData{
			Generated:         time.Now(),
			SessionDebugAvail: sessionProv != nil,
		})
	})

	return mux
}

type indexPageData struct {
	Generated time.Time
	// SessionDebugAvail is true when the underlying *bigtable.Client
	// returned a non-nil SessionDebugProvider. When false, the index
	// renders a banner explaining that the session-scoped views
	// (sessionz / afez / loadz) will show "not enabled" until the
	// caller flips bigtable.ClientConfig.EnableDebug=true.
	SessionDebugAvail bool
}

const indexTplSrc = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>bigtable debug</title>
<style>
body{font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;margin:2em;color:#222;background:#fafafa}
h1{font-size:1.3em;margin:0 0 .3em 0}
h2{font-size:1em;color:#666;font-weight:normal;margin:0 0 1.5em 0}
ul{list-style:none;padding:0;max-width:38em}
li{background:#fff;margin-bottom:.6em;padding:.6em .9em;box-shadow:0 1px 2px rgba(0,0,0,.06)}
li a{color:#1a5fb4;text-decoration:none;font-weight:600;font-size:1em}
li a:hover{text-decoration:underline}
li .desc{color:#666;font-size:.88em;margin-top:.15em}
.disabled{background:#fff8dc;border-left:4px solid #d4a017;padding:.8em 1em;margin:0 0 1.2em 0;max-width:38em;font-size:.92em;color:#5a4a10}
.disabled code{background:#fff2c8;padding:1px 4px;border-radius:2px}
</style>
</head><body>
<h1>Bigtable debug views</h1>
<h2>generated {{.Generated.Format "15:04:05 MST"}}</h2>
{{if not .SessionDebugAvail}}
<div class="disabled">
<strong>session debug not enabled.</strong> Session pool snapshot state
(sessionz / afez / loadz) is not being collected. Set
<code>bigtable.ClientConfig.EnableDebug = true</code> and rebuild
your client to enable per-pool snapshots; leaving it off skips every
allocating debug recorder for zero hot-path overhead.
</div>
{{end}}
<ul>
<li><a href="sessionz/">sessionz</a> <div class="desc">per-pool sessions, states, latency histograms, slow-vRPC log, scaling history.</div></li>
<li><a href="afez/">afez</a> <div class="desc">per-AFE bucketing: refCount, idle, in-use, EWMAs, last-connected.</div></li>
<li><a href="loadz/">loadz</a> <div class="desc">picker decision reasoning, actual-vs-ideal AFE fanout, K-choice trace.</div></li>
<li><a href="channelz/">channelz</a> <div class="desc">gRPC channel pool state (classic + session).</div></li>
<li><a href="configz/">configz</a> <div class="desc">server-driven client configuration (GetClientConfiguration).</div></li>
<li><a href="tcpz/">tcpz</a> <div class="desc">per-connection TCP_INFO (RTT, retrans, PMTU) — requires ClientConfig.EnableDebug=true.</div></li>
<li><a href="debugtagsz/">debugtagsz</a> <div class="desc">counters for "shouldn't happen" events in the session pool / vRPC dispatch / config poll — wrong-state transitions, dropped GOAWAYs, orphaned responses, watchdog kills.</div></li>
</ul>
</body></html>
`

var indexTpl = template.Must(template.New("debugview-index").Parse(indexTplSrc))
