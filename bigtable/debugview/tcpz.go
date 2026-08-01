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

// tcpz view — per-connection TCP_INFO (RTT, retransmits, cwnd, MSS, TCP
// state) for every gRPC dial a bigtable client made through
// bigtable.TCPStats.
//
// Linux only. On other platforms every row surfaces "tcp_info not
// supported on this platform" in the Err column. Not compatible with
// DirectPath (xDS bypasses the standard dialer, so nothing is captured).

package debugview

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/bigtable"
	btransport "cloud.google.com/go/bigtable/internal/transport"
)

func newTcpzHandler(stats *bigtable.TCPStats) http.Handler {
	mux := http.NewServeMux()
	srv := &tcpzServer{stats: stats}
	mux.HandleFunc("/", srv.handleIndex)
	return mux
}

type tcpzServer struct {
	stats *bigtable.TCPStats
}

func (s *tcpzServer) snapshot() []btransport.TCPInfoSnapshot {
	if s.stats == nil {
		return nil
	}
	return s.stats.Snapshot()
}

func (s *tcpzServer) deadConns() []btransport.DeadConnInfo {
	if s.stats == nil {
		return nil
	}
	return s.stats.DeadConns()
}

// tcpzSeverity ranks conns by how much the kernel's TCP_INFO says is
// wrong with them. Ordering is exploited by the sort (higher first) and
// by the row-color CSS classes below.
type tcpzSeverity int

const (
	sevOK   tcpzSeverity = iota // healthy, no signal
	sevNote                     // interesting but not a problem (draining, unreadable)
	sevWarn                     // any real loss/retrans/ECN/reord signal
	sevCrit                     // RTO-driven Loss state or currently backing off
)

// classify inspects one TCP_INFO snapshot and returns its severity plus
// a short "why" list of the specific signals that triggered it. Empty
// list ↔ sevOK.
//
// Rules (highest wins):
//   - crit: CAState=Loss OR Backoff>0 — RTO-driven loss or currently timing out
//   - warn: CAState ∈ {Disorder, CWR, Recovery} OR any non-zero retrans /
//     lost / dsack / ECN / reordering counter
//   - note: State != ESTABLISHED (closing/draining) OR Err populated
//     (couldn't read info — usually non-Linux)
//   - ok:   everything else
func classify(r btransport.TCPInfoSnapshot) (tcpzSeverity, []string) {
	if r.Err != "" {
		return sevNote, []string{"unreadable"}
	}
	var why []string
	sev := sevOK
	bump := func(s tcpzSeverity, tag string) {
		if s > sev {
			sev = s
		}
		why = append(why, tag)
	}

	if r.CAState == "Loss" {
		bump(sevCrit, "Loss")
	}
	if r.Backoff > 0 {
		bump(sevCrit, "backoff")
	}
	switch r.CAState {
	case "Recovery":
		bump(sevWarn, "Recovery")
	case "CWR":
		bump(sevWarn, "CWR")
	case "Disorder":
		bump(sevWarn, "Disorder")
	}
	if r.Retransmits > 0 {
		bump(sevWarn, "retrans")
	}
	if r.TotalRetrans > 0 && r.Retransmits == 0 {
		bump(sevWarn, "past-retrans")
	}
	if r.Lost > 0 {
		bump(sevWarn, "lost")
	}
	if r.DsackDups > 0 {
		bump(sevWarn, "dsack")
	}
	if r.DeliveredCE > 0 {
		bump(sevWarn, "ECN")
	}
	if r.ReordSeen > 0 {
		bump(sevWarn, "reord")
	}

	if sev == sevOK && r.State != "" && r.State != "ESTABLISHED" {
		bump(sevNote, r.State)
	}
	return sev, why
}

// rowClass returns the CSS class for the row background.
func (s tcpzSeverity) rowClass() string {
	switch s {
	case sevCrit:
		return "row-crit"
	case sevWarn:
		return "row-warn"
	case sevNote:
		return "row-note"
	}
	return "row-ok"
}

// durBucket / pctBucket define a single bucket in a histogram: any value
// with v < Max lands here. The final bucket in a table uses Max == 0 as
// "no upper bound" — the ">X" catch-all. Keeping the sentinel as a plain
// zero avoids threading a bool per bucket.
type durBucket struct {
	Max   time.Duration
	Label string
}

type pctBucket struct {
	Max   float64
	Label string
}

// Bucket tables tuned for the shapes we actually see on prod bigtable
// dials: sub-ms RTT, near-zero retrans rates, minutes-to-hours conn
// lifetimes. Log-ish spacing so a single hot outlier doesn't collapse
// every other bar into a hairline.
var rttHistBuckets = []durBucket{
	{200 * time.Microsecond, "<200µs"},
	{500 * time.Microsecond, "200µs-500µs"},
	{time.Millisecond, "500µs-1ms"},
	{2 * time.Millisecond, "1-2ms"},
	{5 * time.Millisecond, "2-5ms"},
	{10 * time.Millisecond, "5-10ms"},
	{25 * time.Millisecond, "10-25ms"},
	{50 * time.Millisecond, "25-50ms"},
	{100 * time.Millisecond, "50-100ms"},
	{500 * time.Millisecond, "100-500ms"},
	{0, ">500ms"},
}

var retransHistBuckets = []pctBucket{
	{0.0000001, "0%"}, // exact-zero bucket; any positive value falls past this
	{0.01, "0-0.01%"},
	{0.05, "0.01-0.05%"},
	{0.1, "0.05-0.1%"},
	{0.5, "0.1-0.5%"},
	{1, "0.5-1%"},
	{5, "1-5%"},
	{0, ">5%"},
}

var ageHistBuckets = []durBucket{
	{5 * time.Second, "<5s"},
	{30 * time.Second, "5-30s"},
	{time.Minute, "30s-1m"},
	{5 * time.Minute, "1-5m"},
	{15 * time.Minute, "5-15m"},
	{time.Hour, "15m-1h"},
	{6 * time.Hour, "1-6h"},
	{24 * time.Hour, "6-24h"},
	{0, ">24h"},
}

// histBar is one row of a rendered histogram: label + up to two stacked
// bars (live/dead) + count(s). LivePct/DeadPct are 0-100 widths relative
// to the panel's tallest bucket so the visual peak always saturates.
type histBar struct {
	Label   string
	Live    int
	Dead    int // only populated for the age panel
	LivePct int
	DeadPct int
}

// histPanel is one rendered histogram panel above the tcpz table.
// Summary is a "n=… p50=… max=…" one-liner shown under the title.
type histPanel struct {
	Title   string
	Bars    []histBar
	Summary string
	HasDead bool // renders the small "live/dead" legend swatch under the panel
}

// bucketizeDur returns per-bucket counts for a slice of durations. Zero
// values are counted (they land in the first bucket, matching the "<200µs"
// / "<5s" semantics).
func bucketizeDur(values []time.Duration, buckets []durBucket) []int {
	counts := make([]int, len(buckets))
	for _, v := range values {
		for i, b := range buckets {
			if b.Max == 0 || v < b.Max {
				counts[i]++
				break
			}
		}
	}
	return counts
}

// bucketizePct is the retrans-ratio companion to bucketizeDur. The first
// bucket has an epsilon Max so exact-zero conns get their own bar (they
// dominate healthy fleets and would otherwise saturate everything else).
func bucketizePct(values []float64, buckets []pctBucket) []int {
	counts := make([]int, len(buckets))
	for _, v := range values {
		for i, b := range buckets {
			if b.Max == 0 || v < b.Max {
				counts[i]++
				break
			}
		}
	}
	return counts
}

// makeHistPanel converts raw live/dead bucket counts into the rendered
// panel struct. Bar widths are normalized against the tallest single
// (live+dead) bar in the panel so the peak always fills the track. Empty
// panels return nil so the template can hide them cleanly.
func makeHistPanel(title, summary string, labels []string, live, dead []int) *histPanel {
	if len(live) != len(labels) {
		return nil
	}
	max := 0
	for i := range labels {
		total := live[i]
		if dead != nil {
			total += dead[i]
		}
		if total > max {
			max = total
		}
	}
	if max == 0 {
		return nil
	}
	bars := make([]histBar, len(labels))
	for i, lbl := range labels {
		b := histBar{Label: lbl, Live: live[i]}
		b.LivePct = live[i] * 100 / max
		if dead != nil {
			b.Dead = dead[i]
			b.DeadPct = dead[i] * 100 / max
		}
		bars[i] = b
	}
	return &histPanel{Title: title, Bars: bars, Summary: summary, HasDead: dead != nil}
}

// percentileDur returns the 0..100 percentile of a duration slice using
// nearest-rank. Non-mutating (sorts a copy). Empty slice → 0.
func percentileDur(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (p * (len(sorted) - 1)) / 100
	return sorted[idx]
}

// percentilePct is the float64 analogue of percentileDur.
func percentilePct(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := (p * (len(sorted) - 1)) / 100
	return sorted[idx]
}

// bucketLabels extracts the .Label field from a bucket table so
// makeHistPanel doesn't need two type-specific overloads.
func durBucketLabels(bs []durBucket) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Label
	}
	return out
}
func pctBucketLabels(bs []pctBucket) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Label
	}
	return out
}

// buildHistPanels computes the three summary panels shown above the tcpz
// table. Called once per HTML render (skipped for JSON responses).
//
//   - RTT: live conns only, kernel-reported smoothed RTT.
//   - Retrans ratio: live conns only, BytesRetrans/BytesSent from the
//     precomputed RetransRatioPct.
//   - Age: live conns' age-so-far AND dead conns' age-at-death, stacked
//     in the same buckets — dead is what the user asked for (how long
//     conns lived before dying); live is the companion (what's alive now).
func buildHistPanels(live []btransport.TCPInfoSnapshot, dead []btransport.DeadConnInfo) []*histPanel {
	now := time.Now()

	var rtts []time.Duration
	var retrans []float64
	var liveAges []time.Duration
	for _, s := range live {
		if s.Err != "" {
			continue
		}
		if s.RTT > 0 {
			rtts = append(rtts, s.RTT)
		}
		if s.BytesSent > 0 {
			retrans = append(retrans, s.RetransRatioPct)
		}
		if !s.DialedAt.IsZero() {
			liveAges = append(liveAges, now.Sub(s.DialedAt))
		}
	}
	deadAges := make([]time.Duration, 0, len(dead))
	for _, d := range dead {
		if d.DialedAt.IsZero() || d.DiedAt.IsZero() {
			continue
		}
		deadAges = append(deadAges, d.DiedAt.Sub(d.DialedAt))
	}

	var panels []*histPanel

	if len(rtts) > 0 {
		counts := bucketizeDur(rtts, rttHistBuckets)
		summary := fmt.Sprintf("n=%d · p50=%s · p90=%s · max=%s",
			len(rtts),
			roundRTTShort(percentileDur(rtts, 50)),
			roundRTTShort(percentileDur(rtts, 90)),
			roundRTTShort(percentileDur(rtts, 100)))
		panels = append(panels, makeHistPanel("RTT", summary, durBucketLabels(rttHistBuckets), counts, nil))
	}
	if len(retrans) > 0 {
		counts := bucketizePct(retrans, retransHistBuckets)
		summary := fmt.Sprintf("n=%d · p50=%s · p90=%s · max=%s",
			len(retrans),
			formatPct(percentilePct(retrans, 50)),
			formatPct(percentilePct(retrans, 90)),
			formatPct(percentilePct(retrans, 100)))
		panels = append(panels, makeHistPanel("Retrans ratio", summary, pctBucketLabels(retransHistBuckets), counts, nil))
	}
	if len(liveAges)+len(deadAges) > 0 {
		liveCounts := bucketizeDur(liveAges, ageHistBuckets)
		deadCounts := bucketizeDur(deadAges, ageHistBuckets)
		var maxLifetime time.Duration
		if len(deadAges) > 0 {
			maxLifetime = percentileDur(deadAges, 100)
		}
		summary := fmt.Sprintf("live=%d · dead=%d", len(liveAges), len(deadAges))
		if len(deadAges) > 0 {
			summary += fmt.Sprintf(" · dead p50=%s max=%s",
				roundDurationShort(percentileDur(deadAges, 50)),
				roundDurationShort(maxLifetime))
		}
		panels = append(panels, makeHistPanel("Conn age", summary, durBucketLabels(ageHistBuckets), liveCounts, deadCounts))
	}
	return panels
}

// roundRTTShort renders sub-ms and sub-second RTT durations with sensible
// precision for the histogram summary line. time.Duration.String() is too
// verbose ("1.234567ms") and roundDurationShort is tuned for minutes+.
func roundRTTShort(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// formatPct renders a small percent value compactly for histogram
// summaries — mirrors the "pct" template func but returns a plain string
// (no HTML wrapping).
func formatPct(v float64) string {
	if v == 0 {
		return "0%"
	}
	if v < 0.01 {
		return "<0.01%"
	}
	return fmt.Sprintf("%.2f%%", v)
}

// tcpzRow bundles a snapshot with its precomputed severity so the
// template doesn't have to re-classify on every template action.
type tcpzRow struct {
	btransport.TCPInfoSnapshot
	Sev      string // rowClass string ("row-crit", …)
	SevRank  int    // 0..3 matching sevOK..sevCrit; used for group-worst rollups
	Why      string // joined why list, e.g. "Loss+backoff+lost"
	Interest bool   // sev > sevOK — the "N interesting" count uses this
}

// anomalyChip captures one non-zero "interesting counter" to render as
// a small colored badge on the one-line per-conn row. Emitting only
// non-zero counters keeps the row scannable — healthy conns are near-empty.
type anomalyChip struct {
	Label string // e.g. "retr", "dsack", "ECN"
	Value string // formatted count / duration
	Sev   string // "warn" | "crit"
}

// anomalies returns the per-conn interesting-counter chips in a stable
// order. Values that are zero (the healthy case) are omitted.
func (r tcpzRow) Anomalies() []anomalyChip {
	var out []anomalyChip
	add := func(label string, v uint32, sev string) {
		if v == 0 {
			return
		}
		out = append(out, anomalyChip{Label: label, Value: fmt.Sprintf("%d", v), Sev: sev})
	}
	addDur := func(label string, d time.Duration, sev string) {
		if d == 0 {
			return
		}
		out = append(out, anomalyChip{Label: label, Value: d.String(), Sev: sev})
	}
	if r.Backoff > 0 {
		add("bkoff", r.Backoff, "crit")
	}
	add("retr", r.Retransmits, "warn")
	if r.Retransmits == 0 && r.TotalRetrans > 0 {
		add("past-retr", r.TotalRetrans, "warn")
	}
	add("lost", r.Lost, "warn")
	add("dsack", r.DsackDups, "warn")
	add("ECN", r.DeliveredCE, "warn")
	add("reord", r.ReordSeen, "warn")
	add("probes", r.Probes, "warn")
	addDur("rwndLim", r.RwndLimited, "warn")
	addDur("sndLim", r.SndbufLimited, "warn")
	if r.NotsentBytes > 0 {
		out = append(out, anomalyChip{Label: "notsent", Value: fmt.Sprintf("%dB", r.NotsentBytes), Sev: "warn"})
	}
	return out
}

// peerGroup aggregates every conn to the same RemoteAddr into one
// rendered card. The default view groups by remote so operators can tell
// "one AFE is misbehaving" apart from "one conn is misbehaving." Fields
// on the group are worst-case rollups across its conns.
type peerGroup struct {
	Remote     string
	Conns      []tcpzRow
	WorstSev   string        // "row-crit" | "row-warn" | "row-note" | "row-ok"
	N          int           // len(Conns) after filters
	NHidden    int           // conns filtered out (e.g. only=hot); dimmed count in the summary
	SumDelRate uint64        // Σ DeliveryRate — current outbound bandwidth to this remote
	SumAvgOut  float64       // Σ lifetime BytesAcked/age (bytes/sec)
	SumAvgIn   float64       // Σ lifetime BytesReceived/age (bytes/sec)
	MaxRTT     time.Duration // worst smoothed RTT in the group
	MaxRtxPct  float64       // worst retrans ratio in the group
	OldestAge  time.Duration // oldest conn's age — proxy for "how long this pool has been open"
	Interest   int           // count of conns with sev > sevOK
}

// buildPeerGroups groups rows by RemoteAddr, sorted by group severity
// desc (crit → warn → note → ok) then by oldest age. Within a group,
// conns are severity-desc then dial-order (newest first). Empty rows
// slice → nil groups.
func buildPeerGroups(rows []tcpzRow, hiddenByRemote map[string]int) []*peerGroup {
	if len(rows) == 0 {
		return nil
	}
	byRemote := make(map[string]*peerGroup)
	order := make([]string, 0, len(rows))
	for _, r := range rows {
		g, ok := byRemote[r.RemoteAddr]
		if !ok {
			g = &peerGroup{Remote: r.RemoteAddr}
			byRemote[r.RemoteAddr] = g
			order = append(order, r.RemoteAddr)
		}
		g.Conns = append(g.Conns, r)
		g.N++
		if r.Interest {
			g.Interest++
		}
		if r.Err == "" {
			g.SumDelRate += r.DeliveryRate
			g.SumAvgOut += avgRateBps(r.BytesAcked, r.DialedAt)
			g.SumAvgIn += avgRateBps(r.BytesReceived, r.DialedAt)
			if r.RTT > g.MaxRTT {
				g.MaxRTT = r.RTT
			}
			if r.RetransRatioPct > g.MaxRtxPct {
				g.MaxRtxPct = r.RetransRatioPct
			}
		}
		if !r.DialedAt.IsZero() {
			if age := time.Since(r.DialedAt); age > g.OldestAge {
				g.OldestAge = age
			}
		}
	}
	// Finalize: pick worst-sev + hidden count, sort conns within each.
	groups := make([]*peerGroup, 0, len(order))
	for _, remote := range order {
		g := byRemote[remote]
		worst := 0
		for _, c := range g.Conns {
			if c.SevRank > worst {
				worst = c.SevRank
			}
		}
		g.WorstSev = sevClassByRank(worst)
		g.NHidden = hiddenByRemote[remote]
		sort.SliceStable(g.Conns, func(i, j int) bool {
			if g.Conns[i].SevRank != g.Conns[j].SevRank {
				return g.Conns[i].SevRank > g.Conns[j].SevRank
			}
			return g.Conns[i].DialedAt.After(g.Conns[j].DialedAt)
		})
		groups = append(groups, g)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		ri := sevRank(groups[i].WorstSev)
		rj := sevRank(groups[j].WorstSev)
		if ri != rj {
			return ri > rj
		}
		if groups[i].Interest != groups[j].Interest {
			return groups[i].Interest > groups[j].Interest
		}
		return groups[i].OldestAge > groups[j].OldestAge
	})
	return groups
}

// sevClassByRank maps a numeric severity rank back to the row-class
// string. Inverse of sevRank; used by peerGroup.WorstSev.
func sevClassByRank(rank int) string {
	switch rank {
	case 3:
		return "row-crit"
	case 2:
		return "row-warn"
	case 1:
		return "row-note"
	}
	return "row-ok"
}

// tcpzColDef describes one column of the tcpz table: header label,
// tooltip, CSS class for both th/td, and (if sortable) a comparator
// returning <0/0/>0 in the natural ascending sense. The "Body" field is
// the exact template snippet used inside the row's <td> — kept
// alongside the header so adding a column is a single-slice-entry
// change.
type tcpzColDef struct {
	Key   string // URL sort key; empty = not sortable
	Label string
	Class string // "num" / "mono" / "err" / ""
	Title string
	Body  string // inner-cell template action, e.g. `{{num .MSS}}`
	Cmp   func(a, b btransport.TCPInfoSnapshot) int
	Desc  bool
}

// tcpzCols is the single source of truth for every column in the tcpz
// table. The template renders <thead> and <tbody> by iterating this
// slice. Order here is the display order.
var tcpzCols = []tcpzColDef{
	{Key: "", Label: "Why", Class: "why", Title: "Why this row is highlighted — the list of TCP_INFO signals that classified it.", Body: `{{.Why}}`},
	{Key: "remote", Label: "Remote", Class: "mono", Title: "Peer address (ip:port).", Body: `{{.RemoteAddr}}`, Cmp: cmpStr(func(s btransport.TCPInfoSnapshot) string { return s.RemoteAddr })},
	{Key: "local", Label: "Local", Class: "mono", Title: "Local socket address.", Body: `{{.LocalAddr}}`, Cmp: cmpStr(func(s btransport.TCPInfoSnapshot) string { return s.LocalAddr })},
	{Key: "age", Label: "Age", Title: "Time since this conn was dialed.", Body: `{{ago .DialedAt}}`, Cmp: cmpTime(func(s btransport.TCPInfoSnapshot) time.Time { return s.DialedAt }), Desc: true},
	{Key: "state", Label: "State", Title: "Linux TCP state (ESTABLISHED, CLOSE_WAIT, etc.).", Body: `<span class="state-{{or .State "UNKNOWN"}}">{{or .State "—"}}</span>`, Cmp: cmpStr(func(s btransport.TCPInfoSnapshot) string { return s.State })},
	{Key: "ca", Label: "Ca", Title: "Congestion-control state: Open=healthy, Disorder=watching for loss, CWR=cwnd-reducing after ECN, Recovery=fast-retransmitting, Loss=RTO-driven collapse (worst).", Body: `<span class="ca-{{or .CAState "Unknown"}}">{{or .CAState "—"}}</span>`, Cmp: cmpStr(func(s btransport.TCPInfoSnapshot) string { return s.CAState })},
	{Key: "bkoff", Label: "Bkoff", Class: "num", Title: "RTO backoff count. >0 means we've timed out at least once and are waiting exponentially longer.", Body: `{{critNum .Backoff}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.Backoff }), Desc: true},
	{Key: "rtt", Label: "RTT", Class: "num", Title: "Smoothed round-trip time — the primary 'wire' latency signal.", Body: `{{dur .RTT}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.RTT }), Desc: true},
	{Key: "rttvar", Label: "RTTVar", Class: "num", Title: "RTT variance (jitter). High values suggest an unstable path.", Body: `{{dur .RTTVar}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.RTTVar }), Desc: true},
	{Key: "minrtt", Label: "MinRTT", Class: "num", Title: "Minimum RTT observed on this conn — the floor of what the network can deliver.", Body: `{{dur .MinRTT}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.MinRTT }), Desc: true},
	{Key: "rto", Label: "RTO", Class: "num", Title: "Current retransmit timeout — kernel will re-send unacked bytes after this long. Grows with backoff.", Body: `{{dur .RTO}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.RTO }), Desc: true},
	{Key: "mss", Label: "MSS", Class: "num", Title: "Send MSS (max segment size).", Body: `{{num .MSS}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.MSS }), Desc: true},
	{Key: "pmtu", Label: "PMTU", Class: "num", Title: "Path MTU (bytes). <1500 = tunneling/VPN in path. Watch for PMTU black holes: silent drops if ICMP frag-needed replies are filtered.", Body: `{{pmtu .PMTU}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.PMTU }), Desc: false},
	{Key: "cwnd", Label: "CWnd", Class: "num", Title: "Send congestion window in MSS units.", Body: `{{num .SndCwnd}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.SndCwnd }), Desc: true},
	{Key: "ssth", Label: "SSTh", Class: "num", Title: "Slow-start threshold in MSS units. When cwnd &lt; ssthresh we're in slow-start (often after a loss event).", Body: `{{num .SndSsthresh}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.SndSsthresh }), Desc: true},
	{Key: "sndwnd", Label: "SndWnd", Class: "num", Title: "Peer's advertised receive window (bytes) — the ceiling on what we can put in flight. Small = the peer is throttling us.", Body: `{{win .SndWnd}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.SndWnd }), Desc: true},
	{Key: "rcvwnd", Label: "RcvWnd", Class: "num", Title: "Our advertised receive window (bytes) — the ceiling the peer can push at us. Shrinking = we're a slow reader.", Body: `{{win .RcvWnd}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.RcvWnd }), Desc: true},
	{Key: "retr", Label: "Retr", Class: "num", Title: "Recent retransmits (kernel counter).", Body: `{{hotNum .Retransmits}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.Retransmits }), Desc: true},
	{Key: "totalretr", Label: "TotalRetr", Class: "num", Title: "Total retransmits since conn open — high count means path is lossy.", Body: `{{hotNum .TotalRetrans}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.TotalRetrans }), Desc: true},
	{Key: "rtxrate", Label: "RtxRate", Class: "num", Title: "Retransmit ratio: bytes retransmitted / bytes sent. The 'actual loss rate' this conn observed.", Body: `{{hotPct .RetransRatioPct}}`, Cmp: cmpF64(func(s btransport.TCPInfoSnapshot) float64 { return s.RetransRatioPct }), Desc: true},
	{Key: "lost", Label: "Lost", Class: "num", Title: "Segments the kernel considers lost right now.", Body: `{{hotNum .Lost}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.Lost }), Desc: true},
	{Key: "sackd", Label: "SACKd", Class: "num", Title: "Segments selectively-ACK'd by the receiver.", Body: `{{num .Sacked}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.Sacked }), Desc: true},
	{Key: "unacked", Label: "Unacked", Class: "num", Title: "Segments sent but not yet ACKed (in flight).", Body: `{{num .Unacked}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.Unacked }), Desc: true},
	{Key: "dsack", Label: "DSACK", Class: "num", Title: "Duplicate-SACK count — number of SPURIOUS retransmits (we resent bytes the receiver actually got). High DSACK relative to TotalRetr = timing false-positives, not real loss.", Body: `{{hotNum .DsackDups}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.DsackDups }), Desc: true},
	{Key: "reord", Label: "ReordS", Class: "num", Title: "Times reordering was observed. Reordering can trigger fast-retransmit even without loss.", Body: `{{hotNum .ReordSeen}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.ReordSeen }), Desc: true},
	{Key: "ecn", Label: "ECN", Class: "num", Title: "Packets delivered with ECN Congestion-Experienced marks. Non-zero = a router is signaling congestion before dropping.", Body: `{{hotNum .DeliveredCE}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.DeliveredCE }), Desc: true},
	{Key: "sent", Label: "Sent", Class: "num", Title: "Total data bytes SENT (outbound cumulative).", Body: `{{bytes .BytesSent}}`, Cmp: cmpU64(func(s btransport.TCPInfoSnapshot) uint64 { return s.BytesSent }), Desc: true},
	{Key: "recv", Label: "Recv", Class: "num", Title: "Total data bytes RECEIVED (inbound cumulative).", Body: `{{bytes .BytesReceived}}`, Cmp: cmpU64(func(s btransport.TCPInfoSnapshot) uint64 { return s.BytesReceived }), Desc: true},
	{Key: "retrans", Label: "Retrans", Class: "num", Title: "Total bytes retransmitted (data).", Body: `{{hotBytes .BytesRetrans}}`, Cmp: cmpU64(func(s btransport.TCPInfoSnapshot) uint64 { return s.BytesRetrans }), Desc: true},
	{Key: "delrate", Label: "DelRate", Class: "num", Title: "Recent outbound delivery rate — kernel's BBR estimate (bytes/sec) of the rate ACKs are flowing back. Best 'current bandwidth OUT' signal we have.", Body: `{{rate .DeliveryRate}}`, Cmp: cmpU64(func(s btransport.TCPInfoSnapshot) uint64 { return s.DeliveryRate }), Desc: true},
	{Key: "avgout", Label: "AvgOut", Class: "num", Title: "Lifetime OUTBOUND throughput: BytesAcked ÷ conn age. Complements DelRate (instantaneous, kernel-windowed) — steady traffic makes them converge, bursty traffic makes AvgOut lag.", Body: `{{avgRate .BytesAcked .DialedAt}}`, Cmp: cmpF64(func(s btransport.TCPInfoSnapshot) float64 { return avgRateBps(s.BytesAcked, s.DialedAt) }), Desc: true},
	{Key: "avgin", Label: "AvgIn", Class: "num", Title: "Lifetime INBOUND throughput: BytesReceived ÷ conn age. TCP_INFO has no windowed inbound-rate field, so lifetime avg is the cheapest honest answer — a windowed value would require diffing snapshots per conn.", Body: `{{avgRate .BytesReceived .DialedAt}}`, Cmp: cmpF64(func(s btransport.TCPInfoSnapshot) float64 { return avgRateBps(s.BytesReceived, s.DialedAt) }), Desc: true},
	{Key: "notsent", Label: "NotSent", Class: "num", Title: "Bytes buffered but not yet on wire. High = we're app-limited or CPU-limited, not network-limited.", Body: `{{num .NotsentBytes}}`, Cmp: cmpU32(func(s btransport.TCPInfoSnapshot) uint32 { return s.NotsentBytes }), Desc: true},
	{Key: "rwndlim", Label: "RwndLim", Class: "num", Title: "Cumulative time the sender was blocked by the peer's receive window. Non-zero = flow control cost real wall clock; the peer is a slow reader.", Body: `{{hotDur .RwndLimited}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.RwndLimited }), Desc: true},
	{Key: "sndbuflim", Label: "SndBufLim", Class: "num", Title: "Cumulative time blocked by our own SO_SNDBUF. Non-zero = our local send buffer is undersized for the bandwidth-delay product.", Body: `{{hotDur .SndbufLimited}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.SndbufLimited }), Desc: true},
	{Key: "lastrecv", Label: "LastRecv", Title: "Time since the socket last received data.", Body: `{{dur .LastDataRecv}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.LastDataRecv }), Desc: true},
	{Key: "lastsent", Label: "LastSent", Title: "Time since the socket last sent data.", Body: `{{dur .LastDataSent}}`, Cmp: cmpDur(func(s btransport.TCPInfoSnapshot) time.Duration { return s.LastDataSent }), Desc: true},
	{Key: "", Label: "Err", Class: "err", Title: "Populated when TCP_INFO couldn't be read on a live fd (e.g. non-Linux OS).", Body: `{{.Err}}`},
}

// tcpzColByKey builds a lookup for tcpzCols; done once at init so the
// request handler doesn't re-scan the slice per request.
var tcpzColByKey = func() map[string]*tcpzColDef {
	m := make(map[string]*tcpzColDef, len(tcpzCols))
	for i := range tcpzCols {
		if tcpzCols[i].Key != "" {
			m[tcpzCols[i].Key] = &tcpzCols[i]
		}
	}
	return m
}()

// cmp* factories: thin wrappers that turn "extract field X" into a
// comparator. Keeps the tcpzColDef literals dense and readable.
func cmpStr(get func(btransport.TCPInfoSnapshot) string) func(a, b btransport.TCPInfoSnapshot) int {
	return func(a, b btransport.TCPInfoSnapshot) int { return strings.Compare(get(a), get(b)) }
}
func cmpU32(get func(btransport.TCPInfoSnapshot) uint32) func(a, b btransport.TCPInfoSnapshot) int {
	return func(a, b btransport.TCPInfoSnapshot) int {
		x, y := get(a), get(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
}
func cmpU64(get func(btransport.TCPInfoSnapshot) uint64) func(a, b btransport.TCPInfoSnapshot) int {
	return func(a, b btransport.TCPInfoSnapshot) int {
		x, y := get(a), get(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
}
func cmpF64(get func(btransport.TCPInfoSnapshot) float64) func(a, b btransport.TCPInfoSnapshot) int {
	return func(a, b btransport.TCPInfoSnapshot) int {
		x, y := get(a), get(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
}
func cmpDur(get func(btransport.TCPInfoSnapshot) time.Duration) func(a, b btransport.TCPInfoSnapshot) int {
	return func(a, b btransport.TCPInfoSnapshot) int {
		x, y := get(a), get(b)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
}
// avgRateBps returns lifetime throughput in bytes/sec (BytesAcked ÷
// age). Zero when the conn hasn't ACKed data yet, isn't dialed, or
// age is non-positive. Used both by the display func and the column
// comparator so sort order matches the rendered value.
func avgRateBps(bytesAcked uint64, dialedAt time.Time) float64 {
	if bytesAcked == 0 || dialedAt.IsZero() {
		return 0
	}
	age := time.Since(dialedAt).Seconds()
	if age <= 0 {
		return 0
	}
	return float64(bytesAcked) / age
}

// formatBytesPerSec renders a bytes/sec value with a scale suffix.
// Shared by the "rate" (kernel-reported DeliveryRate) and "avgRate"
// (BytesAcked ÷ age) template funcs so both columns format identically.
func formatBytesPerSec(v float64) string {
	if v <= 0 {
		return "—"
	}
	switch {
	case v >= 1<<20:
		return fmt.Sprintf("%.1f MB/s", v/(1<<20))
	case v >= 1<<10:
		return fmt.Sprintf("%.1f KB/s", v/(1<<10))
	default:
		return fmt.Sprintf("%.0f B/s", v)
	}
}

func cmpTime(get func(btransport.TCPInfoSnapshot) time.Time) func(a, b btransport.TCPInfoSnapshot) int {
	return func(a, b btransport.TCPInfoSnapshot) int {
		x, y := get(a), get(b)
		switch {
		case x.Before(y):
			return -1
		case x.After(y):
			return 1
		}
		return 0
	}
}

// tcpzHeaderCell is the per-column view struct the template iterates.
// Built once per request so all the "which arrow, which link" logic
// lives in Go — the template just prints strings.
type tcpzHeaderCell struct {
	Label     string
	Class     string
	Title     string
	Href      string        // "" if not sortable
	Arrow     string        // "" / "↑" / "↓"
	BodyTpl   template.HTML // <td>…</td> inner action, wrapped with class
	CellClass string        // repeated per row via BodyTpl already; kept for possible reuse
}

func (s *tcpzServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	all := q.Get("all") == "1"
	onlyHot := q.Get("only") == "hot"
	flat := q.Get("flat") == "1"
	expandAll := q.Get("expand") == "all"
	remoteFilter := q.Get("remote")
	sortKey, sortDir := parseSort(q)

	raw := s.snapshot()
	total := len(raw)
	hidden := 0
	if remoteFilter != "" {
		filtered := raw[:0]
		for _, snap := range raw {
			if snap.RemoteAddr != remoteFilter {
				hidden++
				continue
			}
			filtered = append(filtered, snap)
		}
		raw = filtered
	} else if !all {
		filtered := raw[:0]
		for _, snap := range raw {
			if strings.HasSuffix(snap.RemoteAddr, ":443") {
				hidden++
				continue
			}
			filtered = append(filtered, snap)
		}
		raw = filtered
	}

	// Serve JSON before we build display-only fields.
	if q.Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(raw)
		return
	}

	rows := make([]tcpzRow, 0, len(raw))
	interesting := 0
	dropped := 0
	// hiddenByRemote tracks conns dropped by ?only=hot per remote, so
	// each group can show "N shown · M hidden" without under-representing
	// active-but-quiet peers.
	hiddenByRemote := map[string]int{}
	for _, snap := range raw {
		sev, why := classify(snap)
		if sev > sevOK {
			interesting++
		}
		if onlyHot && sev < sevWarn {
			dropped++
			hiddenByRemote[snap.RemoteAddr]++
			continue
		}
		rows = append(rows, tcpzRow{
			TCPInfoSnapshot: snap,
			Sev:             sev.rowClass(),
			SevRank:         int(sev),
			Why:             strings.Join(why, "+"),
			Interest:        sev > sevOK,
		})
	}

	sortRows(rows, sortKey, sortDir)
	groups := buildPeerGroups(rows, hiddenByRemote)

	// Histograms cover the pre-filter live snapshot (raw) plus every
	// remembered dead conn, so the summary reflects the fleet — the "only
	// hot" / :443-hidden table filters shouldn't be able to hide a hot
	// cluster from the distribution.
	histPanels := buildHistPanels(raw, s.deadConns())

	baseParams := url.Values{}
	if all {
		baseParams.Set("all", "1")
	}
	if onlyHot {
		baseParams.Set("only", "hot")
	}
	if flat {
		// Sort links must round-trip the flat toggle; otherwise clicking
		// a column header jumps back to the grouped default view.
		baseParams.Set("flat", "1")
	}
	headers := make([]tcpzHeaderCell, len(tcpzCols))
	for i, c := range tcpzCols {
		hc := tcpzHeaderCell{Label: c.Label, Class: c.Class, Title: c.Title}
		if c.Cmp != nil {
			nextDir := "asc"
			if c.Desc {
				nextDir = "desc"
			}
			if sortKey == c.Key {
				if sortDir == "asc" {
					hc.Arrow = "↑"
					nextDir = "desc"
				} else {
					hc.Arrow = "↓"
					nextDir = "asc"
				}
			}
			p := cloneValues(baseParams)
			p.Set("sort", c.Key)
			p.Set("dir", nextDir)
			hc.Href = "?" + p.Encode()
		}
		headers[i] = hc
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data := struct {
		Rows        []tcpzRow
		Groups      []*peerGroup
		Cols        []tcpzColDef
		Headers     []tcpzHeaderCell
		HistPanels  []*histPanel
		Count       int
		GroupCount  int
		Total       int
		Hidden      int
		Interesting int
		Dropped     int
		ShowAll     bool
		OnlyHot     bool
		Flat        bool
		ExpandAll   bool
		SortKey     string
		SortDir     string
		SortByDial  bool
		Generated   time.Time
	}{
		Rows:        rows,
		Groups:      groups,
		Cols:        tcpzCols,
		Headers:     headers,
		HistPanels:  histPanels,
		Count:       len(rows),
		GroupCount:  len(groups),
		Total:       total,
		Hidden:      hidden,
		Interesting: interesting,
		Dropped:     dropped,
		ShowAll:     all,
		OnlyHot:     onlyHot,
		Flat:        flat,
		ExpandAll:   expandAll,
		SortKey:     sortKey,
		SortDir:     sortDir,
		SortByDial:  sortKey == "dial",
		Generated:   time.Now(),
	}
	tpl := tcpzTpl
	if flat {
		tpl = tcpzFlatTpl
	}
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// parseSort normalizes ?sort=<key>&dir=<asc|desc>. Unknown keys silently
// fall back to "sev" so a stale bookmarked URL doesn't 404. dir="" means
// "use the column's natural direction" (handled by sortRows).
func parseSort(q url.Values) (key, dir string) {
	key = q.Get("sort")
	dir = q.Get("dir")
	if dir != "asc" && dir != "desc" {
		dir = ""
	}
	switch key {
	case "", "sev", "dial":
		if key == "" {
			key = "sev"
		}
		return key, dir
	}
	if _, ok := tcpzColByKey[key]; !ok {
		return "sev", "" // unknown column — fall back cleanly
	}
	return key, dir
}

// sortRows reorders rows in place per key+dir. Special keys "sev" and
// "dial" don't map to columns and get their own comparators. All other
// keys map to a tcpzColDef.Cmp.
func sortRows(rows []tcpzRow, key, dir string) {
	switch key {
	case "sev":
		sort.SliceStable(rows, func(i, j int) bool {
			si := sevRank(rows[i].Sev)
			sj := sevRank(rows[j].Sev)
			if si != sj {
				return si > sj
			}
			return rows[i].DialedAt.Before(rows[j].DialedAt)
		})
		return
	case "dial":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].DialedAt.Before(rows[j].DialedAt)
		})
		return
	}
	c, ok := tcpzColByKey[key]
	if !ok || c.Cmp == nil {
		return
	}
	desc := c.Desc
	if dir == "asc" {
		desc = false
	} else if dir == "desc" {
		desc = true
	}
	sort.SliceStable(rows, func(i, j int) bool {
		r := c.Cmp(rows[i].TCPInfoSnapshot, rows[j].TCPInfoSnapshot)
		if r == 0 {
			return rows[i].DialedAt.Before(rows[j].DialedAt)
		}
		if desc {
			return r > 0
		}
		return r < 0
	})
}

// cloneValues returns a shallow copy of v so we can mutate it per header
// link without polluting the base params slice.
func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

// sevRank maps the row-class string back to a comparable int so the
// sort comparator doesn't have to hold onto the severity enum
// separately.
func sevRank(cls string) int {
	switch cls {
	case "row-crit":
		return 3
	case "row-warn":
		return 2
	case "row-note":
		return 1
	}
	return 0
}

// tcpzBodyTpls holds each column's cell-body snippet, parsed once at
// package init against the same funcs map the outer template uses. The
// outer template's per-row loop calls {{cell $i $row}} which executes
// the matching bodyTpl — this keeps the outer template short and lets
// column order / additions be a one-slice-entry change to tcpzCols.
var tcpzBodyTpls []*template.Template

func tcpzFuncs() template.FuncMap {
	return template.FuncMap{
		"dur": func(d time.Duration) string {
			if d == 0 {
				return "—"
			}
			return d.String()
		},
		"ago": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return time.Since(t).Round(time.Second).String()
		},
		"num": func(v uint32) string {
			if v == 0 {
				return "—"
			}
			return fmt.Sprintf("%d", v)
		},
		"pmtu": func(v uint32) template.HTML {
			if v == 0 {
				return template.HTML("—")
			}
			switch {
			case v >= 1500:
				return template.HTML(fmt.Sprintf("%d", v))
			case v < 1300:
				return template.HTML(fmt.Sprintf(`<b class="hot" title="%d B below 1500 — unusually heavy encapsulation">%d</b>`, 1500-v, v))
			default:
				return template.HTML(fmt.Sprintf(`<span title="%d B below 1500 — tunneling in path">%d</span>`, 1500-v, v))
			}
		},
		"num64": func(v uint64) string {
			if v == 0 {
				return "—"
			}
			return fmt.Sprintf("%d", v)
		},
		"pct": func(v float64) string {
			if v == 0 {
				return "—"
			}
			if v < 0.01 {
				return "<0.01%"
			}
			return fmt.Sprintf("%.2f%%", v)
		},
		"rate": func(v uint64) string {
			if v == 0 {
				return "—"
			}
			return formatBytesPerSec(float64(v))
		},
		"avgRate": func(bytesAcked uint64, dialedAt time.Time) string {
			return formatBytesPerSec(avgRateBps(bytesAcked, dialedAt))
		},
		"bytes": func(v uint64) string {
			if v == 0 {
				return "—"
			}
			f := float64(v)
			switch {
			case f >= 1<<30:
				return fmt.Sprintf("%.1f GiB", f/(1<<30))
			case f >= 1<<20:
				return fmt.Sprintf("%.1f MiB", f/(1<<20))
			case f >= 1<<10:
				return fmt.Sprintf("%.1f KiB", f/(1<<10))
			default:
				return fmt.Sprintf("%d B", v)
			}
		},
		"hotNum": func(v uint32) template.HTML {
			if v == 0 {
				return template.HTML("—")
			}
			return template.HTML(fmt.Sprintf(`<b class="hot">%d</b>`, v))
		},
		"hotDur": func(d time.Duration) template.HTML {
			if d == 0 {
				return template.HTML("—")
			}
			return template.HTML(fmt.Sprintf(`<b class="hot">%s</b>`, d.String()))
		},
		"win": func(v uint32) string {
			if v == 0 {
				return "—"
			}
			f := float64(v)
			switch {
			case f >= 1<<20:
				return fmt.Sprintf("%.1f MiB", f/(1<<20))
			case f >= 1<<10:
				return fmt.Sprintf("%.1f KiB", f/(1<<10))
			default:
				return fmt.Sprintf("%d B", v)
			}
		},
		"critNum": func(v uint32) template.HTML {
			if v == 0 {
				return template.HTML("—")
			}
			return template.HTML(fmt.Sprintf(`<b class="hot crit">%d</b>`, v))
		},
		"hotBytes": func(v uint64) template.HTML {
			if v == 0 {
				return template.HTML("—")
			}
			f := float64(v)
			var s string
			switch {
			case f >= 1<<30:
				s = fmt.Sprintf("%.1f GiB", f/(1<<30))
			case f >= 1<<20:
				s = fmt.Sprintf("%.1f MiB", f/(1<<20))
			case f >= 1<<10:
				s = fmt.Sprintf("%.1f KiB", f/(1<<10))
			default:
				s = fmt.Sprintf("%d B", v)
			}
			return template.HTML(fmt.Sprintf(`<b class="hot">%s</b>`, s))
		},
		"hotPct": func(v float64) template.HTML {
			if v == 0 {
				return template.HTML("—")
			}
			var s string
			if v < 0.01 {
				s = "<0.01%"
			} else {
				s = fmt.Sprintf("%.2f%%", v)
			}
			return template.HTML(fmt.Sprintf(`<b class="hot">%s</b>`, s))
		},
		"or": func(s, fallback string) string {
			if s == "" {
				return fallback
			}
			return s
		},
		// cell renders one column's inner HTML for one row by executing
		// the per-column body template. tcpzBodyTpls is populated in
		// init(); Parse only needs the func to exist, not for bodies to
		// be ready yet.
		"cell": func(idx int, r tcpzRow) (template.HTML, error) {
			var buf strings.Builder
			if err := tcpzBodyTpls[idx].Execute(&buf, r); err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		},
		// bpsF: format a float64 bytes/sec (aggregate) with the same
		// scale suffix as the per-conn "rate" func. Zero → dash.
		"bpsF": func(v float64) string {
			if v <= 0 {
				return "—"
			}
			return formatBytesPerSec(v)
		},
		// ageDur: pretty-print a positive duration; used by peer-group
		// summaries where we've already computed age with time.Since.
		"ageDur": func(d time.Duration) string {
			if d <= 0 {
				return "—"
			}
			return d.Round(time.Second).String()
		},
	}
}

// Cache the funcs so column bodies parse against the exact same map the
// outer template uses.
var tcpzFuncsCached = tcpzFuncs()

func init() {
	tcpzBodyTpls = make([]*template.Template, len(tcpzCols))
	for i, c := range tcpzCols {
		t, err := template.New(fmt.Sprintf("tcpz-col-%d", i)).Funcs(tcpzFuncsCached).Parse(c.Body)
		if err != nil {
			panic(fmt.Sprintf("tcpz: parse column %q body: %v", c.Label, err))
		}
		tcpzBodyTpls[i] = t
	}
}

// tcpzFlatTpl renders the classic wide-table view (?flat=1). Kept as an
// escape hatch for muscle-memory bookmarks; the default is the grouped
// card view (tcpzTpl below).
var tcpzFlatTpl = template.Must(template.New("tcpz-flat").Funcs(tcpzFuncsCached).Parse(tcpzFlatTplSrc))

const tcpzFlatTplSrc = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>tcpz — {{.Count}} conns{{if .Interesting}} · {{.Interesting}} hot{{end}}</title>
<meta http-equiv="refresh" content="5">
<style>
body { font: 13px/1.4 -apple-system, "Segoe UI", Helvetica, Arial, sans-serif; margin: 1em; color: #222; }
h1 { font-size: 1.1em; margin: 0 0 .3em 0; }
.meta { color: #666; margin-bottom: .3em; }
.legend { color: #666; margin-bottom: .8em; font-size: 12px; }
.legend .sw { display: inline-block; width: .85em; height: .85em; vertical-align: -1px; border: 1px solid #ccc; margin-right: 3px; }
.legend .sw.crit { background: #fdecea; border-color: #f2c2bd; }
.legend .sw.warn { background: #fff5e0; border-color: #ecd9a3; }
.legend .sw.note { background: #eef4fb; border-color: #cddceb; }
.legend .sw.ok   { background: #ffffff; }
table { border-collapse: collapse; width: 100%; }
th, td { text-align: left; padding: 4px 8px; border-bottom: 1px solid #eee; }
th { background: #f4f4f4; font-weight: 600; position: sticky; top: 0; }
td.num { text-align: right; font-variant-numeric: tabular-nums; }
td.mono, th.mono { font-family: SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; }
tr.row-crit td { background: #fdecea; }
tr.row-warn td { background: #fff5e0; }
tr.row-note td { background: #eef4fb; }
tr.row-crit:hover td { background: #fadbd7; }
tr.row-warn:hover td { background: #ffecc4; }
tr.row-note:hover td { background: #dde9f4; }
tr.row-ok:hover td   { background: #fafafa; }
.why { color: #666; font-size: 11px; font-family: SFMono-Regular, Menlo, Consolas, monospace; }
tr.row-crit .why { color: #b32222; }
tr.row-warn .why { color: #a04500; }
b.hot { color: #a04500; }
b.hot.crit { color: #b32222; background: #f7d7d3; padding: 0 3px; border-radius: 2px; }
a.col-sort { color: inherit; text-decoration: none; }
a.col-sort:hover { text-decoration: underline; }
a.col-sort .arr { color: #d95700; margin-left: 2px; }
.empty { color: #888; margin: 2em 0; }
.err { color: #a04500; }
.state-ESTABLISHED { color: #197a1f; }
.state-CLOSE_WAIT, .state-FIN_WAIT1, .state-FIN_WAIT2, .state-CLOSING, .state-LAST_ACK, .state-TIME_WAIT { color: #a04500; font-weight: 600; }
.ca-Open { color: #197a1f; }
.ca-Disorder { color: #a06a00; font-weight: 600; }
.ca-CWR { color: #a04500; font-weight: 600; }
.ca-Recovery { color: #b32222; font-weight: 600; }
.ca-Loss { color: #b32222; font-weight: 700; }
.hist-grid { display: flex; gap: 1em; flex-wrap: wrap; margin: .4em 0 1em 0; }
.hist-panel { flex: 1 1 320px; min-width: 280px; border: 1px solid #e8e8e8; border-radius: 3px; padding: .5em .7em; background: #fafafa; }
.hist-title { font-weight: 600; margin-bottom: .1em; font-size: 12px; }
.hist-summary { color: #666; font-size: 11px; font-variant-numeric: tabular-nums; margin-bottom: .35em; }
.hist-row { display: grid; grid-template-columns: 82px 1fr 60px; gap: 6px; align-items: center; font-size: 11px; font-variant-numeric: tabular-nums; line-height: 1.35; }
.hist-lbl { color: #555; font-family: SFMono-Regular, Menlo, Consolas, monospace; text-align: right; }
.hist-track { position: relative; height: 10px; background: #eee; border-radius: 2px; overflow: hidden; }
.hist-bar-live { position: absolute; top: 0; bottom: 0; left: 0; background: #4a7fbf; }
.hist-bar-dead { position: absolute; top: 0; bottom: 0; background: #b8763f; opacity: .85; }
.hist-cnt { color: #444; font-family: SFMono-Regular, Menlo, Consolas, monospace; }
.hist-cnt .dead { color: #a04500; }
.hist-legend { color: #666; font-size: 11px; margin-top: .4em; }
.hist-legend .sw { display: inline-block; width: .75em; height: .75em; vertical-align: -1px; border-radius: 2px; margin: 0 3px 0 6px; }
.hist-legend .sw.live { background: #4a7fbf; }
.hist-legend .sw.dead { background: #b8763f; }
</style>
</head>
<body>
<h1>tcpz — {{.Count}} conn{{if ne .Count 1}}s{{end}}{{if .Interesting}} · <span style="color:#b32222">{{.Interesting}} interesting</span>{{end}}{{if .Hidden}} <span style="color:#888;font-weight:400;font-size:.85em">({{.Hidden}} :443 hidden)</span>{{end}}{{if .Dropped}} <span style="color:#888;font-weight:400;font-size:.85em">({{.Dropped}} healthy hidden)</span>{{end}}</h1>
<div class="meta">Snapshot at {{.Generated.Format "15:04:05.000"}} · auto-refresh 5s · <a href="?format=json{{if .ShowAll}}&amp;all=1{{end}}">JSON</a>
 · <a href="?{{if .ShowAll}}all=1{{end}}{{if .OnlyHot}}{{if .ShowAll}}&amp;{{end}}only=hot{{end}}">grouped view</a>
{{if .ShowAll}} · <a href="?flat=1">hide :443 (default)</a>{{else if .Hidden}} · <a href="?flat=1&amp;all=1">show all ({{.Total}})</a>{{end}}
{{if .OnlyHot}} · <a href="?flat=1{{if .ShowAll}}&amp;all=1{{end}}">show healthy too</a>{{else}} · <a href="?flat=1&amp;only=hot{{if .ShowAll}}&amp;all=1{{end}}">only hot</a>{{end}}
· sort: {{if eq .SortKey "sev"}}<b>severity</b>{{else}}<a href="?flat=1&amp;{{if .ShowAll}}all=1&amp;{{end}}{{if .OnlyHot}}only=hot&amp;{{end}}sort=sev">severity</a>{{end}}
| {{if eq .SortKey "dial"}}<b>dial order</b>{{else}}<a href="?flat=1&amp;{{if .ShowAll}}all=1&amp;{{end}}{{if .OnlyHot}}only=hot&amp;{{end}}sort=dial">dial order</a>{{end}}
{{if and (ne .SortKey "sev") (ne .SortKey "dial")}} | column: <b>{{.SortKey}} {{if eq .SortDir "asc"}}↑{{else}}↓{{end}}</b>{{end}}
</div>
<div class="legend">Row color:
<span class="sw crit"></span>Loss / backoff (RTO-driven)
<span class="sw warn"></span>retrans / DSACK / ECN / reorder
<span class="sw note"></span>non-ESTABLISHED / unreadable
<span class="sw ok"></span>healthy
· <b class="hot">bold orange</b> = the specific counter that flagged the row.
</div>
{{if .HistPanels}}
<div class="hist-grid">
{{range $panel := .HistPanels}}
<div class="hist-panel">
<div class="hist-title">{{$panel.Title}}</div>
<div class="hist-summary">{{$panel.Summary}}</div>
{{range $panel.Bars}}
<div class="hist-row">
<span class="hist-lbl">{{.Label}}</span>
<span class="hist-track">
{{if .LivePct}}<span class="hist-bar-live" style="width:{{.LivePct}}%"></span>{{end}}
{{if .DeadPct}}<span class="hist-bar-dead" style="left:{{.LivePct}}%;width:{{.DeadPct}}%"></span>{{end}}
</span>
<span class="hist-cnt">{{.Live}}{{if $panel.HasDead}} / <span class="dead">{{.Dead}}</span>{{end}}</span>
</div>
{{end}}
{{if $panel.HasDead}}<div class="hist-legend"><span class="sw live"></span>live<span class="sw dead"></span>dead</div>{{end}}
</div>
{{end}}
</div>
{{end}}
{{if not .Rows}}
<div class="empty">No conns registered. Either the client uses DirectPath (xDS bypasses the standard dialer, so nothing is captured), no traffic has been dialed yet, {{if .OnlyHot}}every conn is healthy (try <a href="?">without ?only=hot</a>), {{end}}or bigtable.TCPStats was never passed into the Client's options.</div>
{{else}}
<table>
<thead><tr>
{{range .Headers}}
<th{{if .Class}} class="{{.Class}}"{{end}} title="{{.Title}}">{{if .Href}}<a class="col-sort" href="{{.Href}}">{{.Label}}{{if .Arrow}}<span class="arr">{{.Arrow}}</span>{{end}}</a>{{else}}{{.Label}}{{end}}</th>
{{end}}
</tr></thead>
<tbody>
{{range $r := .Rows}}
<tr class="{{$r.Sev}}">{{range $i, $c := $.Cols}}<td{{if $c.Class}} class="{{$c.Class}}"{{end}}>{{cell $i $r}}</td>{{end}}</tr>
{{end}}
</tbody>
</table>
{{end}}
</body>
</html>
`

// tcpzTpl is the default grouped view: one card per remote peer, with
// a one-line summary per conn expandable to six field cards
// (Traffic / Latency / Loss / Window / RTO / Idle). Designed so a
// healthy fleet is a wall of green with tiny numbers, and one hot conn
// turns its whole card orange with the specific counter that flagged
// it visible without expanding anything.
var tcpzTpl = template.Must(template.New("tcpz-grouped").Funcs(tcpzFuncsCached).Parse(tcpzTplSrc))

const tcpzTplSrc = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>tcpz — {{.GroupCount}} peer{{if ne .GroupCount 1}}s{{end}}{{if .Interesting}} · {{.Interesting}} hot{{end}}</title>
<meta http-equiv="refresh" content="5">
<style>
body { font: 13px/1.4 -apple-system, "Segoe UI", Helvetica, Arial, sans-serif; margin: 1em; color: #222; background: #fafbfc; }
h1 { font-size: 1.15em; margin: 0 0 .3em 0; }
.meta { color: #666; margin-bottom: .5em; }
.meta a { color: #2f5f9f; text-decoration: none; }
.meta a:hover { text-decoration: underline; }
.legend { color: #666; margin-bottom: .8em; font-size: 12px; }
.legend .sw { display: inline-block; width: .85em; height: .85em; vertical-align: -1px; border-radius: 2px; border: 1px solid #ccc; margin-right: 3px; }
.legend .sw.crit { background: #fdecea; border-color: #f2c2bd; }
.legend .sw.warn { background: #fff5e0; border-color: #ecd9a3; }
.legend .sw.note { background: #eef4fb; border-color: #cddceb; }
.legend .sw.ok   { background: #eefaf0; border-color: #cfe9d3; }

/* Peer group card */
.grp { border: 1px solid #dfe3e8; border-radius: 6px; background: #fff; margin: 0 0 .8em 0; overflow: hidden; box-shadow: 0 1px 0 rgba(0,0,0,.03); }
.grp.row-crit { border-color: #f2c2bd; background: #fef7f5; }
.grp.row-warn { border-color: #ecd9a3; background: #fefbf1; }
.grp.row-note { border-color: #cddceb; background: #f6f9fd; }
.grp-hdr { display: flex; align-items: center; gap: .7em; padding: .55em .8em; border-bottom: 1px solid #eee; background: rgba(0,0,0,.015); }
.grp.row-crit .grp-hdr { background: #fbe3df; border-bottom-color: #f2c2bd; }
.grp.row-warn .grp-hdr { background: #fbeecd; border-bottom-color: #ecd9a3; }
.grp.row-note .grp-hdr { background: #e6eefa; border-bottom-color: #cddceb; }
.grp-remote { font-family: SFMono-Regular, Menlo, Consolas, monospace; font-size: 12.5px; font-weight: 600; color: #333; }
.grp-stats { color: #555; font-size: 12px; font-variant-numeric: tabular-nums; margin-left: auto; }
.grp-stats b { color: #222; font-weight: 600; }
.grp-count { color: #444; font-size: 12px; }
.grp-count .hot { color: #b32222; font-weight: 600; }
.grp-count .hidden { color: #888; }

/* Chip: state / CA / anomaly. Consistent shape across all rows. */
.chip { display: inline-block; padding: 1px 6px; border-radius: 10px; font-size: 11px; font-variant-numeric: tabular-nums; border: 1px solid #ccc; background: #f6f6f6; color: #444; }
.chip.state-ESTABLISHED { background: #eefaf0; border-color: #cfe9d3; color: #197a1f; }
.chip.state-CLOSE_WAIT, .chip.state-FIN_WAIT1, .chip.state-FIN_WAIT2, .chip.state-CLOSING, .chip.state-LAST_ACK, .chip.state-TIME_WAIT { background: #fbeecd; border-color: #ecd9a3; color: #a04500; }
.chip.ca-Open { background: #eefaf0; border-color: #cfe9d3; color: #197a1f; }
.chip.ca-Disorder { background: #fbeecd; border-color: #ecd9a3; color: #a04500; }
.chip.ca-CWR, .chip.ca-Recovery { background: #fbe3df; border-color: #f2c2bd; color: #b32222; }
.chip.ca-Loss { background: #fbe3df; border-color: #f2c2bd; color: #b32222; font-weight: 700; }
.chip.anom-warn { background: #fbeecd; border-color: #ecd9a3; color: #a04500; font-weight: 600; }
.chip.anom-crit { background: #fbe3df; border-color: #f2c2bd; color: #b32222; font-weight: 700; }

/* Per-conn one-line summary; details for the expanded field cards. */
details.conn { border-top: 1px solid #eee; }
details.conn:first-of-type { border-top: none; }
details.conn > summary { list-style: none; cursor: pointer; padding: .45em .8em; display: flex; align-items: center; gap: .6em; font-size: 12.5px; }
details.conn > summary::-webkit-details-marker { display: none; }
details.conn > summary::before { content: "▶"; color: #999; font-size: 10px; width: 10px; }
details.conn[open] > summary::before { content: "▼"; }
details.conn > summary:hover { background: rgba(0,0,0,.02); }
.conn-id { font-family: SFMono-Regular, Menlo, Consolas, monospace; color: #555; min-width: 130px; }
.conn-tp { font-family: SFMono-Regular, Menlo, Consolas, monospace; font-variant-numeric: tabular-nums; color: #333; margin-left: auto; }
.conn-tp .up { color: #197a1f; }
.conn-tp .dn { color: #2f5f9f; }
.conn-why { color: #a04500; font-size: 11px; font-family: SFMono-Regular, Menlo, Consolas, monospace; }
details.conn.row-crit > summary { background: rgba(179,34,34,.05); }
details.conn.row-crit .conn-why { color: #b32222; }

/* Field cards inside an expanded conn */
.cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: .5em; padding: .5em .8em .8em .8em; background: rgba(0,0,0,.02); }
.card { background: #fff; border: 1px solid #e2e6ea; border-radius: 4px; padding: .45em .6em; font-size: 12px; }
.card h4 { margin: 0 0 .3em 0; font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: #666; font-weight: 600; }
.card dl { display: grid; grid-template-columns: 1fr auto; gap: 1px 8px; margin: 0; font-variant-numeric: tabular-nums; }
.card dt { color: #555; }
.card dd { margin: 0; color: #222; text-align: right; font-family: SFMono-Regular, Menlo, Consolas, monospace; }
.card dd .hot { color: #a04500; font-weight: 600; }
.card dd .hot.crit { color: #b32222; font-weight: 700; }
.card dd.warn { color: #a04500; font-weight: 600; }
.card dd.crit { color: #b32222; font-weight: 700; }
.card.err-card { border-color: #f2c2bd; background: #fef7f5; grid-column: 1 / -1; }

/* Reuse the histogram panels verbatim from the flat view */
.hist-grid { display: flex; gap: 1em; flex-wrap: wrap; margin: .4em 0 1em 0; }
.hist-panel { flex: 1 1 320px; min-width: 280px; border: 1px solid #e2e6ea; border-radius: 4px; padding: .5em .7em; background: #fff; }
.hist-title { font-weight: 600; margin-bottom: .1em; font-size: 12px; }
.hist-summary { color: #666; font-size: 11px; font-variant-numeric: tabular-nums; margin-bottom: .35em; }
.hist-row { display: grid; grid-template-columns: 82px 1fr 60px; gap: 6px; align-items: center; font-size: 11px; font-variant-numeric: tabular-nums; line-height: 1.35; }
.hist-lbl { color: #555; font-family: SFMono-Regular, Menlo, Consolas, monospace; text-align: right; }
.hist-track { position: relative; height: 10px; background: #f0f0f0; border-radius: 2px; overflow: hidden; }
.hist-bar-live { position: absolute; top: 0; bottom: 0; left: 0; background: #4a7fbf; }
.hist-bar-dead { position: absolute; top: 0; bottom: 0; background: #b8763f; opacity: .85; }
.hist-cnt { color: #444; font-family: SFMono-Regular, Menlo, Consolas, monospace; }
.hist-cnt .dead { color: #a04500; }
.hist-legend { color: #666; font-size: 11px; margin-top: .4em; }
.hist-legend .sw { display: inline-block; width: .75em; height: .75em; vertical-align: -1px; border-radius: 2px; margin: 0 3px 0 6px; }
.hist-legend .sw.live { background: #4a7fbf; }
.hist-legend .sw.dead { background: #b8763f; }

.empty { color: #888; margin: 2em 0; }
</style>
</head>
<body>
<h1>tcpz — {{.GroupCount}} peer{{if ne .GroupCount 1}}s{{end}} · {{.Count}} conn{{if ne .Count 1}}s{{end}}{{if .Interesting}} · <span style="color:#b32222">{{.Interesting}} interesting</span>{{end}}{{if .Hidden}} <span style="color:#888;font-weight:400;font-size:.85em">({{.Hidden}} :443 hidden)</span>{{end}}{{if .Dropped}} <span style="color:#888;font-weight:400;font-size:.85em">({{.Dropped}} healthy hidden)</span>{{end}}</h1>
<div class="meta">Snapshot at {{.Generated.Format "15:04:05.000"}} · auto-refresh 5s
 · <a href="?format=json{{if .ShowAll}}&amp;all=1{{end}}">JSON</a>
{{if .ShowAll}} · <a href="?">hide :443 (default)</a>{{else if .Hidden}} · <a href="?all=1">show all ({{.Total}})</a>{{end}}
{{if .OnlyHot}} · <a href="?{{if .ShowAll}}all=1{{end}}">show healthy too</a>{{else}} · <a href="?only=hot{{if .ShowAll}}&amp;all=1{{end}}">only hot</a>{{end}}
{{if .ExpandAll}} · <a href="?{{if .ShowAll}}all=1{{end}}{{if .OnlyHot}}{{if .ShowAll}}&amp;{{end}}only=hot{{end}}">collapse all</a>{{else}} · <a href="?expand=all{{if .ShowAll}}&amp;all=1{{end}}{{if .OnlyHot}}&amp;only=hot{{end}}">expand all</a>{{end}}
 · <a href="?flat=1{{if .ShowAll}}&amp;all=1{{end}}{{if .OnlyHot}}&amp;only=hot{{end}}">flat view (raw table)</a>
</div>
<div class="legend">Card color by worst conn in the peer group:
<span class="sw crit"></span>Loss / backoff (RTO-driven)
<span class="sw warn"></span>retrans / DSACK / ECN / reorder
<span class="sw note"></span>non-ESTABLISHED / unreadable
<span class="sw ok"></span>healthy
· <span class="chip anom-warn">retr:3</span>/<span class="chip anom-crit">bkoff:2</span> chips on a conn row = non-zero counters worth investigating (zeros stay hidden).
</div>
{{if .HistPanels}}
<div class="hist-grid">
{{range $panel := .HistPanels}}
<div class="hist-panel">
<div class="hist-title">{{$panel.Title}}</div>
<div class="hist-summary">{{$panel.Summary}}</div>
{{range $panel.Bars}}
<div class="hist-row">
<span class="hist-lbl">{{.Label}}</span>
<span class="hist-track">
{{if .LivePct}}<span class="hist-bar-live" style="width:{{.LivePct}}%"></span>{{end}}
{{if .DeadPct}}<span class="hist-bar-dead" style="left:{{.LivePct}}%;width:{{.DeadPct}}%"></span>{{end}}
</span>
<span class="hist-cnt">{{.Live}}{{if $panel.HasDead}} / <span class="dead">{{.Dead}}</span>{{end}}</span>
</div>
{{end}}
{{if $panel.HasDead}}<div class="hist-legend"><span class="sw live"></span>live<span class="sw dead"></span>dead</div>{{end}}
</div>
{{end}}
</div>
{{end}}
{{if not .Groups}}
<div class="empty">No conns registered. Either the client uses DirectPath (xDS bypasses the standard dialer, so nothing is captured), no traffic has been dialed yet, {{if .OnlyHot}}every conn is healthy (try <a href="?">without ?only=hot</a>), {{end}}or bigtable.TCPStats was never passed into the Client's options.</div>
{{else}}
{{range $g := .Groups}}
<div class="grp {{$g.WorstSev}}">
  <div class="grp-hdr">
    <span class="grp-remote">{{$g.Remote}}</span>
    <span class="grp-count">{{$g.N}} conn{{if ne $g.N 1}}s{{end}}{{if $g.Interest}} · <span class="hot">{{$g.Interest}} hot</span>{{end}}{{if $g.NHidden}} · <span class="hidden">{{$g.NHidden}} hidden</span>{{end}}</span>
    <span class="grp-stats">
      {{if $g.SumDelRate}}now <b>{{rate $g.SumDelRate}}</b>{{end}}
      {{if $g.SumAvgOut}} · avg out <b>{{bpsF $g.SumAvgOut}}</b>{{end}}
      {{if $g.SumAvgIn}} · in <b>{{bpsF $g.SumAvgIn}}</b>{{end}}
      {{if $g.MaxRTT}} · worst RTT <b>{{$g.MaxRTT}}</b>{{end}}
      {{if $g.MaxRtxPct}} · worst rtx <b>{{pct $g.MaxRtxPct}}</b>{{end}}
      {{if $g.OldestAge}} · oldest <b>{{ageDur $g.OldestAge}}</b>{{end}}
    </span>
  </div>
  {{range $r := $g.Conns}}
  <details class="conn {{$r.Sev}}"{{if $.ExpandAll}} open{{end}}>
    <summary>
      <span class="conn-id">{{ago $r.DialedAt}} · {{$r.LocalAddr}}</span>
      <span class="chip state-{{or $r.State "UNKNOWN"}}">{{or $r.State "—"}}</span>
      {{if $r.CAState}}{{if ne $r.CAState "Open"}}<span class="chip ca-{{$r.CAState}}">{{$r.CAState}}</span>{{end}}{{end}}
      {{range $a := $r.Anomalies}}<span class="chip anom-{{$a.Sev}}">{{$a.Label}}:{{$a.Value}}</span>{{end}}
      {{if $r.Why}}<span class="conn-why">{{$r.Why}}</span>{{end}}
      <span class="conn-tp">
        {{if $r.DeliveryRate}}<span class="up">↑{{rate $r.DeliveryRate}}</span>{{else}}<span style="color:#aaa">idle</span>{{end}}
      </span>
    </summary>
    <div class="cards">
      <div class="card">
        <h4>Traffic</h4>
        <dl>
          <dt>DelRate</dt><dd>{{rate $r.DeliveryRate}}</dd>
          <dt>AvgOut</dt><dd>{{avgRate $r.BytesAcked $r.DialedAt}}</dd>
          <dt>AvgIn</dt><dd>{{avgRate $r.BytesReceived $r.DialedAt}}</dd>
          <dt>Sent</dt><dd>{{bytes $r.BytesSent}}</dd>
          <dt>Recv</dt><dd>{{bytes $r.BytesReceived}}</dd>
          <dt>NotSent</dt><dd>{{num $r.NotsentBytes}}</dd>
          <dt>Pacing</dt><dd>{{rate $r.PacingRate}}</dd>
        </dl>
      </div>
      <div class="card">
        <h4>Latency</h4>
        <dl>
          <dt>RTT</dt><dd>{{dur $r.RTT}}</dd>
          <dt>RTTVar</dt><dd>{{dur $r.RTTVar}}</dd>
          <dt>MinRTT</dt><dd>{{dur $r.MinRTT}}</dd>
          <dt>RTO</dt><dd>{{dur $r.RTO}}</dd>
          <dt>ATO</dt><dd>{{dur $r.ATO}}</dd>
          <dt>LastRecv</dt><dd>{{dur $r.LastDataRecv}}</dd>
          <dt>LastSent</dt><dd>{{dur $r.LastDataSent}}</dd>
        </dl>
      </div>
      <div class="card">
        <h4>Loss / retrans</h4>
        <dl>
          <dt>Retr</dt><dd>{{hotNum $r.Retransmits}}</dd>
          <dt>TotalRetr</dt><dd>{{hotNum $r.TotalRetrans}}</dd>
          <dt>RtxRate</dt><dd>{{hotPct $r.RetransRatioPct}}</dd>
          <dt>BytesRetrans</dt><dd>{{hotBytes $r.BytesRetrans}}</dd>
          <dt>Lost</dt><dd>{{hotNum $r.Lost}}</dd>
          <dt>SACKd</dt><dd>{{num $r.Sacked}}</dd>
          <dt>DSACK</dt><dd>{{hotNum $r.DsackDups}}</dd>
          <dt>ReordS</dt><dd>{{hotNum $r.ReordSeen}}</dd>
          <dt>ECN</dt><dd>{{hotNum $r.DeliveredCE}}</dd>
        </dl>
      </div>
      <div class="card">
        <h4>Window / MSS</h4>
        <dl>
          <dt>CWnd</dt><dd>{{num $r.SndCwnd}}</dd>
          <dt>SSTh</dt><dd>{{num $r.SndSsthresh}}</dd>
          <dt>SndWnd</dt><dd>{{win $r.SndWnd}}</dd>
          <dt>RcvWnd</dt><dd>{{win $r.RcvWnd}}</dd>
          <dt>Unacked</dt><dd>{{num $r.Unacked}}</dd>
          <dt>MSS</dt><dd>{{num $r.MSS}}</dd>
          <dt>PMTU</dt><dd>{{pmtu $r.PMTU}}</dd>
        </dl>
      </div>
      <div class="card">
        <h4>RTO / probes</h4>
        <dl>
          <dt>Backoff</dt><dd>{{critNum $r.Backoff}}</dd>
          <dt>Probes</dt><dd>{{hotNum $r.Probes}}</dd>
          <dt>TotalRTO</dt><dd>{{hotNum $r.TotalRTO}}</dd>
          <dt>RTOrecov</dt><dd>{{num $r.TotalRTORecoveries}}</dd>
          <dt>RTOtime</dt><dd>{{dur $r.TotalRTOTime}}</dd>
          <dt>Rehash</dt><dd>{{hotNum $r.Rehash}}</dd>
          <dt>RcvOoO</dt><dd>{{num $r.RcvOooPack}}</dd>
        </dl>
      </div>
      <div class="card">
        <h4>Blocking / segs</h4>
        <dl>
          <dt>RwndLim</dt><dd>{{hotDur $r.RwndLimited}}</dd>
          <dt>SndBufLim</dt><dd>{{hotDur $r.SndbufLimited}}</dd>
          <dt>BusyTime</dt><dd>{{dur $r.BusyTime}}</dd>
          <dt>SegsOut</dt><dd>{{num $r.SegsOut}}</dd>
          <dt>SegsIn</dt><dd>{{num $r.SegsIn}}</dd>
          <dt>DataSegsOut</dt><dd>{{num $r.DataSegsOut}}</dd>
          <dt>DataSegsIn</dt><dd>{{num $r.DataSegsIn}}</dd>
        </dl>
      </div>
      {{if $r.Err}}
      <div class="card err-card">
        <h4>Err</h4>
        <div>{{$r.Err}}</div>
      </div>
      {{end}}
    </div>
  </details>
  {{end}}
</div>
{{end}}
{{end}}
</body>
</html>
`
