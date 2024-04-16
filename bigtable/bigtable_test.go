/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc"
)

func TestPrefix(t *testing.T) {
	for _, test := range []struct {
		prefix, succ string
	}{
		{"", ""},
		{"\xff", ""}, // when used, "" means Infinity
		{"x\xff", "y"},
		{"\xfe", "\xff"},
	} {
		got := prefixSuccessor(test.prefix)
		if got != test.succ {
			t.Errorf("prefixSuccessor(%q) = %q, want %s", test.prefix, got, test.succ)
			continue
		}
		r := PrefixRange(test.prefix)
		if test.succ == "" && r.end != "" {
			t.Errorf("PrefixRange(%q) got end %q", test.prefix, r.end)
		}
		if test.succ != "" && r.end != test.succ {
			t.Errorf("PrefixRange(%q) got end %q, want %q", test.prefix, r.end, test.succ)
		}
	}
}

func TestNewClosedOpenRange(t *testing.T) {
	start := "b"
	limit := "b\x01"
	r := NewClosedOpenRange(start, limit)
	for _, test := range []struct {
		k        string
		contains bool
	}{
		{"a", false},
		{"b", true},
		{"b\x00", true},
		{"b\x01", false},
	} {
		if want, got := test.contains, r.Contains(test.k); want != got {
			t.Errorf("%s.Contains(%q) = %t, want %t", r.String(), test.k, got, want)
		}
	}

	for _, test := range []struct {
		start, limit string
		valid        bool
	}{
		{"a", "a", false},
		{"b", "a", false},
		{"a", "a\x00", true},
		{"a", "b", true},
	} {
		r := NewClosedOpenRange(test.start, test.limit)
		if want, got := test.valid, r.valid(); want != got {
			t.Errorf("%s.valid() = %t, want %t", r.String(), got, want)
		}
	}
}
func TestNewOpenClosedRange(t *testing.T) {
	start := "b"
	limit := "b\x01"
	r := NewOpenClosedRange(start, limit)
	for _, test := range []struct {
		k        string
		contains bool
	}{
		{"a", false},
		{"b", false},
		{"b\x00", true},
		{"b\x01", true},
		{"b\x01\x00", false},
	} {
		if want, got := test.contains, r.Contains(test.k); want != got {
			t.Errorf("%s.Contains(%q) = %t, want %t", r.String(), test.k, got, want)
		}
	}

	for _, test := range []struct {
		start, limit string
		valid        bool
	}{
		{"a", "a", false},
		{"b", "a", false},
		{"a", "a\x00", true},
		{"a", "b", true},
	} {
		r := NewOpenClosedRange(test.start, test.limit)
		if want, got := test.valid, r.valid(); want != got {
			t.Errorf("%s.valid() = %t, want %t", r.String(), got, want)
		}
	}
}
func TestNewClosedRange(t *testing.T) {
	start := "b"
	limit := "b"

	r := NewClosedRange(start, limit)
	for _, test := range []struct {
		k        string
		contains bool
	}{
		{"a", false},
		{"b", true},
		{"b\x01", false},
	} {
		if want, got := test.contains, r.Contains(test.k); want != got {
			t.Errorf("NewClosedRange(%q, %q).Contains(%q) = %t, want %t", "a", "a\x01", test.k, got, test.contains)
		}
	}

	for _, test := range []struct {
		start, limit string
		valid        bool
	}{
		{"a", "b", true},
		{"b", "b", true},
		{"b", "b\x00", true},
		{"b\x00", "b", false},
	} {
		r := NewClosedRange(test.start, test.limit)
		if want, got := test.valid, r.valid(); want != got {
			t.Errorf("NewClosedRange(%q, %q).valid() = %t, want %t", test.start, test.limit, got, want)
		}
	}
}

func TestNewOpenRange(t *testing.T) {
	start := "b"
	limit := "b\x01"

	r := NewOpenRange(start, limit)
	for _, test := range []struct {
		k        string
		contains bool
	}{
		{"a", false},
		{"b", false},
		{"b\x00", true},
		{"b\x01", false},
	} {
		if want, got := test.contains, r.Contains(test.k); want != got {
			t.Errorf("NewOpenRange(%q, %q).Contains(%q) = %t, want %t", "a", "a\x01", test.k, got, test.contains)
		}
	}

	for _, test := range []struct {
		start, limit string
		valid        bool
	}{
		{"a", "a", false},
		{"a", "b", true},
		{"a", "a\x00", true},
		{"a", "a\x01", true},
	} {
		r := NewOpenRange(test.start, test.limit)
		if want, got := test.valid, r.valid(); want != got {
			t.Errorf("NewOpenRange(%q, %q).valid() = %t, want %t", test.start, test.limit, got, want)
		}
	}
}

func TestInfiniteRange(t *testing.T) {
	r := InfiniteRange("b")
	for _, test := range []struct {
		k        string
		contains bool
	}{
		{"a", false},
		{"b", true},
		{"b\x00", true},
		{"z", true},
	} {
		if want, got := test.contains, r.Contains(test.k); want != got {
			t.Errorf("%s.Contains(%q) = %t, want %t", r.String(), test.k, got, want)
		}
	}

	for _, test := range []struct {
		start string
		valid bool
	}{
		{"a", true},
		{"", true},
	} {
		r := InfiniteRange(test.start)
		if want, got := test.valid, r.valid(); want != got {
			t.Errorf("%s.valid() = %t, want %t", r.String(), got, want)
		}
	}
}

func TestInfiniteReverseRange(t *testing.T) {
	r := InfiniteReverseRange("z")
	for _, test := range []struct {
		k        string
		contains bool
	}{
		{"a", true},
		{"z", true},
		{"z\x00", false},
	} {
		if want, got := test.contains, r.Contains(test.k); want != got {
			t.Errorf("%s.Contains(%q) = %t, want %t", r.String(), test.k, got, want)
		}
	}

	for _, test := range []struct {
		start string
		valid bool
	}{
		{"a", true},
		{"", true},
	} {
		r := InfiniteReverseRange(test.start)
		if want, got := test.valid, r.valid(); want != got {
			t.Errorf("%s.valid() = %t, want %t", r.String(), got, want)
		}
	}
}

func TestApplyErrors(t *testing.T) {
	ctx := context.Background()
	table := &Table{
		c: &Client{
			project:  "P",
			instance: "I",
		},
		table: "t",
	}
	f := ColumnFilter("C")
	m := NewMutation()
	m.DeleteRow()
	// Test nested conditional mutations.
	cm := NewCondMutation(f, NewCondMutation(f, m, nil), nil)
	if err := table.Apply(ctx, "x", cm); err == nil {
		t.Error("got nil, want error")
	}
	cm = NewCondMutation(f, nil, NewCondMutation(f, m, nil))
	if err := table.Apply(ctx, "x", cm); err == nil {
		t.Error("got nil, want error")
	}
}

func TestGroupEntries(t *testing.T) {
	for _, test := range []struct {
		desc string
		in   []*entryErr
		size int
		want [][]*entryErr
	}{
		{
			desc: "one entry less than max size is one group",
			in:   []*entryErr{buildEntry(5)},
			size: 10,
			want: [][]*entryErr{{buildEntry(5)}},
		},
		{
			desc: "one entry equal to max size is one group",
			in:   []*entryErr{buildEntry(10)},
			size: 10,
			want: [][]*entryErr{{buildEntry(10)}},
		},
		{
			desc: "one entry greater than max size is one group",
			in:   []*entryErr{buildEntry(15)},
			size: 10,
			want: [][]*entryErr{{buildEntry(15)}},
		},
		{
			desc: "all entries fitting within max size are one group",
			in:   []*entryErr{buildEntry(10), buildEntry(10)},
			size: 20,
			want: [][]*entryErr{{buildEntry(10), buildEntry(10)}},
		},
		{
			desc: "entries each under max size and together over max size are grouped separately",
			in:   []*entryErr{buildEntry(10), buildEntry(10)},
			size: 15,
			want: [][]*entryErr{{buildEntry(10)}, {buildEntry(10)}},
		},
		{
			desc: "entries together over max size are grouped by max size",
			in:   []*entryErr{buildEntry(5), buildEntry(5), buildEntry(5)},
			size: 10,
			want: [][]*entryErr{{buildEntry(5), buildEntry(5)}, {buildEntry(5)}},
		},
		{
			desc: "one entry over max size and one entry under max size are two groups",
			in:   []*entryErr{buildEntry(15), buildEntry(5)},
			size: 10,
			want: [][]*entryErr{{buildEntry(15)}, {buildEntry(5)}},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			if got, want := groupEntries(test.in, test.size), test.want; !cmp.Equal(mutationCounts(got), mutationCounts(want)) {
				t.Fatalf("[%s] want = %v, got = %v", test.desc, mutationCounts(want), mutationCounts(got))
			}
		})
	}
}

func buildEntry(numMutations int) *entryErr {
	var muts []*btpb.Mutation
	for i := 0; i < numMutations; i++ {
		muts = append(muts, &btpb.Mutation{})
	}
	return &entryErr{Entry: &btpb.MutateRowsRequest_Entry{Mutations: muts}}
}

func mutationCounts(batched [][]*entryErr) []int {
	var res []int
	for _, entries := range batched {
		var count int
		for _, e := range entries {
			count += len(e.Entry.Mutations)
		}
		res = append(res, count)
	}
	return res
}

type requestCountingInterceptor struct {
	grpc.ClientStream
	requestCallback func()
}

func (i *requestCountingInterceptor) SendMsg(m interface{}) error {
	i.requestCallback()
	return i.ClientStream.SendMsg(m)
}

func (i *requestCountingInterceptor) RecvMsg(m interface{}) error {
	return i.ClientStream.RecvMsg(m)
}

func requestCallback(callback func()) func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		return &requestCountingInterceptor{
			ClientStream:    clientStream,
			requestCallback: callback,
		}, err
	}
}

func TestRowRangeProto(t *testing.T) {

	for _, test := range []struct {
		desc  string
		rr    RowRange
		proto *btpb.RowSet
	}{
		{
			desc: "RowRange proto start and end",
			rr:   NewClosedOpenRange("a", "b"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("b")},
			}}},
		},
		{
			desc: "RowRange proto start but empty end",
			rr:   NewClosedOpenRange("a", ""),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
			}}},
		},
		{
			desc:  "RowRange proto unbound",
			rr:    NewClosedOpenRange("", ""),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{}}},
		},
		{
			desc:  "RowRange proto unbound with no start or end",
			rr:    InfiniteRange(""),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{}}},
		},
		{
			desc: "RowRange proto open closed",
			rr:   NewOpenClosedRange("a", "b"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyOpen{StartKeyOpen: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyClosed{EndKeyClosed: []byte("b")},
			}}},
		},
		{
			desc: "RowRange proto open closed and empty start",
			rr:   NewOpenClosedRange("", "b"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				EndKey: &btpb.RowRange_EndKeyClosed{EndKeyClosed: []byte("b")},
			}}},
		},
		{
			desc: "RowRange proto open closed and empty start",
			rr:   NewOpenClosedRange("", "b"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				EndKey: &btpb.RowRange_EndKeyClosed{EndKeyClosed: []byte("b")},
			}}},
		},
		{
			desc: "RowRange proto closed open",
			rr:   NewClosedOpenRange("a", "b"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("b")},
			}}},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got := test.rr.proto()
			want := test.proto
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Bad proto for %s: got %v, want %v", test.rr.String(), got, want)
			}
		})
	}
}

func TestRowRangeRetainRowsBefore(t *testing.T) {
	for _, test := range []struct {
		desc  string
		rr    RowSet
		proto *btpb.RowSet
	}{
		{
			desc: "retain rows before",
			rr:   NewRange("a", "c").retainRowsBefore("b"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("b")},
			}}},
		},
		{
			desc: "retain rows before empty key",
			rr:   NewRange("a", "c").retainRowsBefore(""),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("c")},
			}}},
		},
		{
			desc: "retain rows before key greater than range end",
			rr:   NewClosedRange("a", "c").retainRowsBefore("d"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyClosed{EndKeyClosed: []byte("c")},
			}}},
		},
		{
			desc: "retain rows before key same as closed end key",
			rr:   NewClosedRange("a", "c").retainRowsBefore("c"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
				EndKey:   &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("c")},
			}}},
		},
		{
			desc: "retain rows before on unbounded range",
			rr:   InfiniteRange("").retainRowsBefore("z"),
			proto: &btpb.RowSet{RowRanges: []*btpb.RowRange{{
				EndKey: &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("z")},
			}}},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got := test.rr.proto()
			want := test.proto
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Bad retain rows before proto: got %v, want %v", got, want)
			}
		})
	}
}

func TestRowRangeString(t *testing.T) {

	for _, test := range []struct {
		desc string
		rr   RowRange
		str  string
	}{
		{
			desc: "RowRange closed open",
			rr:   NewClosedOpenRange("a", "b"),
			str:  "[\"a\",b)",
		},
		{
			desc: "RowRange open open",
			rr:   NewOpenRange("c", "d"),
			str:  "(\"c\",d)",
		},
		{
			desc: "RowRange closed closed",
			rr:   NewClosedRange("e", "f"),
			str:  "[\"e\",f]",
		},
		{
			desc: "RowRange open closed",
			rr:   NewOpenClosedRange("g", "h"),
			str:  "(\"g\",h]",
		},
		{
			desc: "RowRange unbound unbound",
			rr:   InfiniteRange(""),
			str:  "(∞,∞)",
		},
		{
			desc: "RowRange closed unbound",
			rr:   InfiniteRange("b"),
			str:  "[\"b\",∞)",
		},
		{
			desc: "RowRange unbound closed",
			rr:   InfiniteReverseRange("c"),
			str:  "(∞,c]",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got := test.rr.String()
			want := test.str
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Bad String(): got %v, want %v", got, want)
			}
		})
	}
}

// TestReadRowsInvalidRowSet verifies that the client doesn't send ReadRows() requests with invalid RowSets.
func TestReadRowsInvalidRowSet(t *testing.T) {
	testEnv, err := NewEmulatedEnv(IntegrationTestConfig{})
	if err != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", err)
	}
	var requestCount int
	incrementRequestCount := func() { requestCount++ }
	conn, err := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20)),
		grpc.WithStreamInterceptor(requestCallback(incrementRequestCount)),
	)
	if err != nil {
		t.Fatalf("grpc.Dial failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminClient, err := NewAdminClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer adminClient.Close()
	if err := adminClient.CreateTable(ctx, testEnv.config.Table); err != nil {
		t.Fatalf("CreateTable(%v) failed: %v", testEnv.config.Table, err)
	}
	client, err := NewClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()
	table := client.Open(testEnv.config.Table)
	tests := []struct {
		rr    RowSet
		valid bool
	}{
		{
			rr:    RowRange{startBound: rangeUnbounded, endBound: rangeUnbounded},
			valid: true,
		},
		{
			rr:    RowRange{startBound: rangeClosed, start: "b", endBound: rangeUnbounded},
			valid: true,
		},
		{
			rr:    RowRange{startBound: rangeClosed, start: "b", endBound: rangeOpen, end: "c"},
			valid: true,
		},
		{
			rr:    RowRange{startBound: rangeClosed, start: "b", endBound: rangeOpen, end: "a"},
			valid: false,
		},
		{
			rr:    RowList{"a"},
			valid: true,
		},
		{
			rr:    RowList{},
			valid: false,
		},
	}
	for _, test := range tests {
		requestCount = 0
		err = table.ReadRows(ctx, test.rr, func(r Row) bool { return true })
		if err != nil {
			t.Fatalf("ReadRows(%v) failed: %v", test.rr, err)
		}
		requestValid := requestCount != 0
		if requestValid != test.valid {
			t.Errorf("%s: got %v, want %v", test.rr, requestValid, test.valid)
		}
	}
}

func TestReadRowsRequestStats(t *testing.T) {
	testEnv, err := NewEmulatedEnv(IntegrationTestConfig{})
	if err != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", err)
	}
	conn, err := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20)),
	)
	if err != nil {
		t.Fatalf("grpc.Dial failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminClient, err := NewAdminClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer adminClient.Close()
	tableConf := &TableConf{
		TableID: testEnv.config.Table,
		Families: map[string]GCPolicy{
			"f": NoGcPolicy(),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, tableConf); err != nil {
		t.Fatalf("CreateTable(%v) failed: %v", testEnv.config.Table, err)
	}

	client, err := NewClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()
	table := client.Open(testEnv.config.Table)

	m := NewMutation()
	m.Set("f", "q", ServerTime, []byte("value"))

	if err = table.Apply(ctx, "row1", m); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	m = NewMutation()
	m.Set("f", "q", ServerTime, []byte("value"))
	m.Set("f", "q2", ServerTime, []byte("value2"))
	if err = table.Apply(ctx, "row2", m); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	m = NewMutation()
	m.Set("f", "excluded", ServerTime, []byte("value"))
	if err = table.Apply(ctx, "row3", m); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	statsChannel := make(chan FullReadStats, 1)

	readStart := time.Now()
	if err := table.ReadRows(ctx, InfiniteRange(""), func(r Row) bool { return true }, WithFullReadStats(func(s *FullReadStats) { statsChannel <- *s }), RowFilter(ColumnFilter("q.*"))); err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	readElapsed := time.Since(readStart)

	got := <-statsChannel

	wantIter := ReadIterationStats{
		RowsSeenCount:      3,
		RowsReturnedCount:  2,
		CellsSeenCount:     4,
		CellsReturnedCount: 3,
	}

	if diff := cmp.Diff(wantIter, got.ReadIterationStats); diff != "" {
		t.Errorf("ReadRows RequestStats are incorrect (-want +got):\n%s", diff)
	}

	if got.RequestLatencyStats.FrontendServerLatency > readElapsed || got.RequestLatencyStats.FrontendServerLatency <= 0 {
		t.Fatalf("ReadRows FrontendServerLatency should be in range 0, %v", readElapsed)
	}
}

// TestHeaderPopulatedWithAppProfile verifies that request params header is populated with table name and app profile
func TestHeaderPopulatedWithAppProfile(t *testing.T) {
	testEnv, err := NewEmulatedEnv(IntegrationTestConfig{})
	if err != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", err)
	}
	conn, err := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		t.Fatalf("grpc.Dial failed: %v", err)
	}
	ctx := context.Background()
	opt := option.WithGRPCConn(conn)
	config := ClientConfig{
		AppProfile: "my-app-profile",
	}
	client, err := NewClientWithConfig(ctx, "my-project", "my-instance", config, opt)
	if err != nil {
		t.Fatalf("Failed to create client %v", err)
	}
	table := client.Open("my-table")
	if table == nil {
		t.Fatal("Failed to open table")
	}

	resourcePrefixHeaderValue := table.md.Get(resourcePrefixHeader)
	if got, want := len(resourcePrefixHeaderValue), 1; got != want {
		t.Fatalf("Incorrect number of header values in resourcePrefixHeader. Got %d, want %d", got, want)
	}
	if got, want := resourcePrefixHeaderValue[0], "projects/my-project/instances/my-instance/tables/my-table"; got != want {
		t.Errorf("Incorrect value in resourcePrefixHeader. Got %s, want %s", got, want)
	}

	requestParamsHeaderValue := table.md.Get(requestParamsHeader)
	if got, want := len(requestParamsHeaderValue), 1; got != want {
		t.Fatalf("Incorrect number of header values in requestParamsHeader. Got %d, want %d", got, want)
	}
	if got, want := requestParamsHeaderValue[0], "table_name=projects%2Fmy-project%2Finstances%2Fmy-instance%2Ftables%2Fmy-table&app_profile_id=my-app-profile"; got != want {
		t.Errorf("Incorrect value in resourcePrefixHeader. Got %s, want %s", got, want)
	}
}

func TestMutateRowsWithAggregates(t *testing.T) {
	testEnv, err := NewEmulatedEnv(IntegrationTestConfig{})
	if err != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", err)
	}
	conn, err := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20)),
	)
	if err != nil {
		t.Fatalf("grpc.Dial failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminClient, err := NewAdminClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer adminClient.Close()

	tableConf := &TableConf{
		TableID: testEnv.config.Table,
		ColumnFamilies: map[string]Family{
			"f": {
				ValueType: AggregateType{
					Input:      Int64Type{},
					Aggregator: SumAggregator{},
				},
			},
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, tableConf); err != nil {
		t.Fatalf("CreateTable(%v) failed: %v", testEnv.config.Table, err)
	}

	client, err := NewClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()
	table := client.Open(testEnv.config.Table)

	m := NewMutation()
	m.AddIntToCell("f", "q", 0, 1000)
	err = table.Apply(ctx, "row1", m)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	m = NewMutation()
	m.AddIntToCell("f", "q", 0, 2000)
	err = table.Apply(ctx, "row1", m)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	row, err := table.ReadRow(ctx, "row1")
	if !bytes.Equal(row["f"][0].Value, binary.BigEndian.AppendUint64([]byte{}, 3000)) {
		t.Error()
	}
}
