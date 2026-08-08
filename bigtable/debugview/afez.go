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

// afez view — live per-AFE bucketing of every session pool. Groups AFE
// rows by pool: id, ref-count (idle+in-use), idle, in-use, transport
// EWMA, e2e EWMA, last-connected age. AFEs with ref-count == 0 (empty
// buckets awaiting GC) are faded.

package debugview

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// newAfezHandler wires the afez sub-mux for a given SessionDebugProvider.
// Called from Handler; the returned handler still owns its own inner mux
// so /?format=json and other query routing land on the same "/" handler.
func newAfezHandler(p bigtable.SessionDebugProvider) http.Handler {
	mux := http.NewServeMux()
	srv := &afezServer{provider: p}
	mux.HandleFunc("/", srv.handleIndex)
	return mux
}

type afezServer struct {
	provider bigtable.SessionDebugProvider
}

// afezRow is one AFE — the JSON-friendly representation the HTML template
// renders and the ?format=json response emits verbatim. JSON field names
// are stable across the fold.
type afezRow struct {
	Pool          string        `json:"pool"`
	AfeID         int64         `json:"afeId"`
	AfeIDHex      string        `json:"afeIdHex"`
	RefCount      int           `json:"refCount"`
	IdleCount     int           `json:"idleCount"`
	InUseCount    int           `json:"inUseCount"`
	TransportEwma time.Duration `json:"transportEwmaNanos"`
	E2eEwma       time.Duration `json:"e2eEwmaNanos"`
	LastConnected time.Time     `json:"lastConnected"`
	IdleAge       time.Duration `json:"idleAgeNanos"`
	PendingGC     bool          `json:"pendingGC"`
}

type afezPage struct {
	Generated time.Time
	Pools     []afezPoolBlock
	Total     int
}

type afezPoolBlock struct {
	Name string
	Rows []afezRow
}

func (s *afezServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "" {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	pools, total := s.collect(now)

	if r.URL.Query().Get("format") == "json" {
		var flat []afezRow
		for _, pb := range pools {
			flat = append(flat, pb.Rows...)
		}
		writeJSON(w, struct {
			CapturedAt time.Time `json:"capturedAt"`
			AFEs       []afezRow `json:"afes"`
			Total      int       `json:"total"`
		}{now, flat, total})
		return
	}

	writeHTML(w, afezTpl, afezPage{Generated: now, Pools: pools, Total: total})
}

// collect walks every pool snapshot and returns per-pool rows sorted by
// AFE id (sessionList.Snapshot already sorts).
func (s *afezServer) collect(now time.Time) (pools []afezPoolBlock, total int) {
	if s.provider == nil {
		return nil, 0
	}
	for _, p := range s.provider.Snapshot() {
		if len(p.AFEs) == 0 {
			continue
		}
		rows := make([]afezRow, 0, len(p.AFEs))
		for _, a := range p.AFEs {
			inUse := a.RefCount - a.IdleCount
			if inUse < 0 {
				inUse = 0
			}
			var idleAge time.Duration
			if !a.LastConnected.IsZero() {
				idleAge = now.Sub(a.LastConnected)
			}
			rows = append(rows, afezRow{
				Pool:          p.Name,
				AfeID:         int64(a.ID),
				AfeIDHex:      fmt.Sprintf("%x", uint64(a.ID)),
				RefCount:      a.RefCount,
				IdleCount:     a.IdleCount,
				InUseCount:    inUse,
				TransportEwma: a.TransportEwma,
				E2eEwma:       a.E2eEwma,
				LastConnected: a.LastConnected,
				IdleAge:       idleAge,
				PendingGC:     a.RefCount == 0,
			})
			total++
		}
		pools = append(pools, afezPoolBlock{Name: p.Name, Rows: rows})
	}
	return pools, total
}

var _ = btransport.AfeSnapshotRow{} // guard against accidental unused-import trim

func afezFuncs() template.FuncMap {
	m := commonFuncs()
	m["dur"] = func(d time.Duration) string {
		if d == 0 {
			return "—"
		}
		return roundDurationShort(d).String()
	}
	m["age"] = func(d time.Duration) string {
		if d <= 0 {
			return "—"
		}
		return roundDurationShort(d).String()
	}
	return m
}

var afezTpl = template.Must(template.New("afez").Funcs(afezFuncs()).Parse(afezTplSrc))

const afezTplSrc = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="5">
<title>afez</title>
<style>
body { font-family: -apple-system,Segoe UI,Roboto,sans-serif; margin: 1rem; }
h1 { font-size: 1.1rem; margin: 0 0 .5rem; }
h2 { font-size: .95rem; margin: 1.25rem 0 .25rem; color: #444; }
table { border-collapse: collapse; width: 100%; font-size: .85rem; }
th, td { padding: .3rem .55rem; text-align: right; border-bottom: 1px solid #eee; font-variant-numeric: tabular-nums; }
th:first-child, td:first-child { text-align: left; }
th { background: #f6f6f6; font-weight: 600; }
tr.pending-gc td { color: #999; font-style: italic; }
.empty { color: #888; margin: 1rem 0; }
.gen { color: #888; font-size: .8rem; margin-top: 1rem; }
</style>
</head><body>
<h1>Bigtable AFE view — {{.Total}} AFE(s) across {{len .Pools}} pool(s)</h1>
{{if .Pools}}
{{range .Pools}}
<h2>{{.Name}}</h2>
<table>
  <thead><tr>
    <th>AFE id</th><th>ref</th><th>idle</th><th>in-use</th>
    <th>transport EWMA</th><th>e2e EWMA</th><th>last connected</th>
  </tr></thead>
  <tbody>
  {{range .Rows}}
  <tr{{if .PendingGC}} class="pending-gc"{{end}}>
    <td>{{if .AfeID}}0x{{.AfeIDHex}}{{else}}<em>unknown</em>{{end}}</td>
    <td>{{.RefCount}}</td>
    <td>{{.IdleCount}}</td>
    <td>{{.InUseCount}}</td>
    <td>{{dur .TransportEwma}}</td>
    <td>{{dur .E2eEwma}}</td>
    <td>{{age .IdleAge}} ago</td>
  </tr>
  {{end}}
  </tbody>
</table>
{{end}}
{{else}}
<p class="empty">No AFE buckets recorded yet. Sessions may still be handshaking, or session pooling may be disabled.</p>
{{end}}
<p class="gen">Generated {{.Generated.Format "15:04:05.000 MST"}} — auto-refresh 5s.</p>
</body></html>`
