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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// TestTcpz_EmptyRenders confirms the handler serves a valid HTML page
// with the "no conns registered" hint when no dials have happened yet.
// Exercises the template on the empty-slice path (guards against a
// template bug that only surfaces without data).
func TestTcpz_EmptyRenders(t *testing.T) {
	h := newTcpzHandler(bigtable.NewTCPStats())

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html*", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "No conns registered") {
		t.Errorf("empty page did not render the empty-state hint; body:\n%s", body)
	}
}

// TestTcpz_JSON confirms ?format=json returns a JSON array (empty is
// fine) with the right Content-Type. Cheap regression guard for
// template bypass logic.
func TestTcpz_JSON(t *testing.T) {
	h := newTcpzHandler(bigtable.NewTCPStats())

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?format=json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var rows []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatalf("JSON decode: %v — body: %s", err, rr.Body.String())
	}
	if len(rows) != 0 {
		t.Errorf("empty registry → rows = %d, want 0", len(rows))
	}
}

// TestTcpz_NilStatsSafe guards against a panic when a caller wires the
// handler without a TCPStats. Renders as if empty.
func TestTcpz_NilStatsSafe(t *testing.T) {
	h := newTcpzHandler(nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP %d, want 200", rr.Code)
	}
}

// TestTcpz_Classify pins the severity buckets so that a future edit to
// the classifier can't quietly re-color rows. Each row picks the
// *sharpest* signal for that severity — CAState=Loss for crit,
// TotalRetrans>0 for warn, CLOSE_WAIT for note, an all-zero snapshot
// for ok.
func TestTcpz_Classify(t *testing.T) {
	tests := []struct {
		name    string
		snap    btransport.TCPInfoSnapshot
		wantSev tcpzSeverity
		wantWhy string // substring match
	}{
		{
			name:    "healthy Open ESTABLISHED",
			snap:    btransport.TCPInfoSnapshot{CAState: "Open", State: "ESTABLISHED"},
			wantSev: sevOK,
		},
		{
			name:    "unreadable is note not warn",
			snap:    btransport.TCPInfoSnapshot{Err: "tcp_info not supported"},
			wantSev: sevNote,
			wantWhy: "unreadable",
		},
		{
			name:    "CLOSE_WAIT with no loss = note",
			snap:    btransport.TCPInfoSnapshot{State: "CLOSE_WAIT", CAState: "Open"},
			wantSev: sevNote,
			wantWhy: "CLOSE_WAIT",
		},
		{
			name:    "past retrans = warn",
			snap:    btransport.TCPInfoSnapshot{State: "ESTABLISHED", CAState: "Open", TotalRetrans: 5},
			wantSev: sevWarn,
			wantWhy: "past-retrans",
		},
		{
			name:    "DSACK-only = warn",
			snap:    btransport.TCPInfoSnapshot{State: "ESTABLISHED", CAState: "Open", DsackDups: 3},
			wantSev: sevWarn,
			wantWhy: "dsack",
		},
		{
			name:    "ECN-only = warn",
			snap:    btransport.TCPInfoSnapshot{State: "ESTABLISHED", CAState: "Open", DeliveredCE: 2},
			wantSev: sevWarn,
			wantWhy: "ECN",
		},
		{
			name:    "Recovery CA state = warn",
			snap:    btransport.TCPInfoSnapshot{State: "ESTABLISHED", CAState: "Recovery"},
			wantSev: sevWarn,
			wantWhy: "Recovery",
		},
		{
			name:    "Loss CA state = crit",
			snap:    btransport.TCPInfoSnapshot{State: "ESTABLISHED", CAState: "Loss", TotalRetrans: 12},
			wantSev: sevCrit,
			wantWhy: "Loss",
		},
		{
			name:    "backoff>0 = crit regardless of CA",
			snap:    btransport.TCPInfoSnapshot{State: "ESTABLISHED", CAState: "Open", Backoff: 1},
			wantSev: sevCrit,
			wantWhy: "backoff",
		},
		{
			name: "current retrans burst suppresses past-retrans tag",
			snap: btransport.TCPInfoSnapshot{
				State: "ESTABLISHED", CAState: "Open", Retransmits: 2, TotalRetrans: 10,
			},
			wantSev: sevWarn,
			wantWhy: "retrans", // and NOT "past-retrans"
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSev, why := classify(tc.snap)
			if gotSev != tc.wantSev {
				t.Errorf("severity = %v, want %v (why=%v)", gotSev, tc.wantSev, why)
			}
			whyStr := strings.Join(why, "+")
			if tc.wantWhy != "" && !strings.Contains(whyStr, tc.wantWhy) {
				t.Errorf("why = %q, want to contain %q", whyStr, tc.wantWhy)
			}
			// Guard: the "current retrans burst suppresses past-retrans"
			// invariant is subtle enough to deserve its own assertion.
			if tc.name == "current retrans burst suppresses past-retrans tag" {
				if strings.Contains(whyStr, "past-retrans") {
					t.Errorf("why contains past-retrans when Retransmits>0: %q", whyStr)
				}
			}
		})
	}
}

// TestTcpz_ParseSort covers the three shapes: default (sev), known
// column, and unknown-key fallback to sev. Also asserts that a bad
// direction gets squashed to "" so column comparators can fall back to
// their natural direction.
func TestTcpz_ParseSort(t *testing.T) {
	tests := []struct {
		name    string
		q       string
		wantKey string
		wantDir string
	}{
		{"empty defaults to sev", "", "sev", ""},
		{"sev explicit", "sort=sev", "sev", ""},
		{"dial explicit", "sort=dial", "dial", ""},
		{"known column ascending", "sort=rtt&dir=asc", "rtt", "asc"},
		{"known column descending", "sort=totalretr&dir=desc", "totalretr", "desc"},
		{"unknown column falls back to sev", "sort=bogus", "sev", ""},
		{"invalid dir dropped", "sort=rtt&dir=sideways", "rtt", ""},
		{"dir without sort is dropped by fallback", "dir=asc", "sev", "asc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tc.q)
			gotKey, gotDir := parseSort(q)
			if gotKey != tc.wantKey || gotDir != tc.wantDir {
				t.Errorf("parseSort(%q) = %q,%q; want %q,%q", tc.q, gotKey, gotDir, tc.wantKey, tc.wantDir)
			}
		})
	}
}

// TestTcpz_SortRows_byColumn exercises the per-column sort path with a
// small mixed row set. We only assert first/last row identity (peer
// address) — that's the observable behavior clicking a header changes.
func TestTcpz_SortRows_byColumn(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	mk := func(peer string, rtt time.Duration, retrans uint32, dialOffset time.Duration) tcpzRow {
		snap := btransport.TCPInfoSnapshot{
			RemoteAddr: peer, RTT: rtt, TotalRetrans: retrans,
			DialedAt: t0.Add(dialOffset),
			CAState:  "Open", State: "ESTABLISHED",
		}
		sev, why := classify(snap)
		return tcpzRow{
			TCPInfoSnapshot: snap,
			Sev:             sev.rowClass(),
			Why:             strings.Join(why, "+"),
			Interest:        sev > sevOK,
		}
	}
	base := func() []tcpzRow {
		return []tcpzRow{
			mk("a:1", 12*time.Millisecond, 0, 0),
			mk("b:2", 3*time.Millisecond, 5, time.Second),
			mk("c:3", 40*time.Millisecond, 0, 2*time.Second),
			mk("d:4", 8*time.Millisecond, 0, 3*time.Second),
		}
	}

	// rtt desc: c (40ms) first, b (3ms) last.
	rows := base()
	sortRows(rows, "rtt", "desc")
	if rows[0].RemoteAddr != "c:3" || rows[len(rows)-1].RemoteAddr != "b:2" {
		t.Errorf("rtt desc: got %v..%v, want c:3..b:2", rows[0].RemoteAddr, rows[len(rows)-1].RemoteAddr)
	}

	// rtt asc: b (3ms) first.
	rows = base()
	sortRows(rows, "rtt", "asc")
	if rows[0].RemoteAddr != "b:2" {
		t.Errorf("rtt asc: got %v first, want b:2", rows[0].RemoteAddr)
	}

	// totalretr desc: b (5) first; ties (all zero) break by dial order.
	rows = base()
	sortRows(rows, "totalretr", "desc")
	if rows[0].RemoteAddr != "b:2" {
		t.Errorf("totalretr desc: got %v first, want b:2", rows[0].RemoteAddr)
	}
	if rows[1].RemoteAddr != "a:1" || rows[2].RemoteAddr != "c:3" || rows[3].RemoteAddr != "d:4" {
		t.Errorf("totalretr desc tie-break order: got %v,%v,%v; want a:1,c:3,d:4",
			rows[1].RemoteAddr, rows[2].RemoteAddr, rows[3].RemoteAddr)
	}

	// dial special key: pure oldest-first regardless of other fields.
	rows = base()
	sortRows(rows, "dial", "")
	for i, want := range []string{"a:1", "b:2", "c:3", "d:4"} {
		if rows[i].RemoteAddr != want {
			t.Errorf("dial: rows[%d] = %v, want %v", i, rows[i].RemoteAddr, want)
		}
	}

	// unknown key is a no-op — order preserved.
	rows = base()
	sortRows(rows, "no-such-key", "desc")
	if rows[0].RemoteAddr != "a:1" {
		t.Errorf("unknown key: expected no-op, got %v first", rows[0].RemoteAddr)
	}
}

// TestTcpz_SortRows_sevPromotesInteresting confirms that the default
// sev sort puts a Loss-state conn ahead of a healthy conn dialed
// earlier.
func TestTcpz_SortRows_sevPromotesInteresting(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	healthy := btransport.TCPInfoSnapshot{RemoteAddr: "healthy:1", State: "ESTABLISHED", CAState: "Open", DialedAt: t0}
	bad := btransport.TCPInfoSnapshot{RemoteAddr: "bad:2", State: "ESTABLISHED", CAState: "Loss", DialedAt: t0.Add(time.Second)}
	toRow := func(s btransport.TCPInfoSnapshot) tcpzRow {
		sev, why := classify(s)
		return tcpzRow{TCPInfoSnapshot: s, Sev: sev.rowClass(), Why: strings.Join(why, "+"), Interest: sev > sevOK}
	}
	rows := []tcpzRow{toRow(healthy), toRow(bad)}
	sortRows(rows, "sev", "")
	if rows[0].RemoteAddr != "bad:2" {
		t.Errorf("sev sort: healthy conn came before Loss conn (got %v first)", rows[0].RemoteAddr)
	}
}

// TestTcpz_ColumnSortLinksRender is a smoke test that the rendered
// HTML wires the escape-hatch flat view (which still exposes sortable
// columns). The grouped default view sorts by severity implicitly and
// doesn't render sort chrome, so we check `?flat=1` explicitly.
func TestTcpz_ColumnSortLinksRender(t *testing.T) {
	h := newTcpzHandler(bigtable.NewTCPStats())

	// Grouped default: must at least advertise the flat-view escape hatch.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("grouped view HTTP %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, "flat=1") {
		t.Errorf("grouped view missing flat-view switcher; body:\n%s", body)
	}

	// Flat view: must contain the sort meta-bar links (page has no rows
	// with no data, so the header sort links aren't rendered, but the
	// meta-bar sort=sev/sort=dial links always are).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?flat=1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("flat view HTTP %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "sort=sev") && !strings.Contains(body, "sort=dial") {
		t.Errorf("flat view meta bar missing sort links; body:\n%s", body)
	}
}

// TestTcpz_SevRank_matchesRowClass ensures the string-based sort rank
// stays in lockstep with the severity enum. If someone adds a new
// severity, this test flags the missing case before it becomes a silent
// mis-sort.
func TestTcpz_SevRank_matchesRowClass(t *testing.T) {
	pairs := []struct {
		sev tcpzSeverity
	}{
		{sevOK}, {sevNote}, {sevWarn}, {sevCrit},
	}
	prev := -1
	for _, p := range pairs {
		got := sevRank(p.sev.rowClass())
		if got <= prev {
			t.Errorf("sevRank(%v) = %d, expected strictly increasing (prev=%d)", p.sev, got, prev)
		}
		prev = got
	}
}
