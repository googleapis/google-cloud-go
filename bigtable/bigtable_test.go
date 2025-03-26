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
	"errors"
	"hash/crc32"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/civil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var disableMetricsConfig = ClientConfig{MetricsProvider: NoopMetricsProvider{}}

var emulatorUnsupported = "the emulator does not currently support"

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
			project:              "P",
			instance:             "I",
			metricsTracerFactory: &builtinMetricsTracerFactory{},
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
			str:  "[\"a\",\"b\")",
		},
		{
			desc: "RowRange open open",
			rr:   NewOpenRange("c", "d"),
			str:  "(\"c\",\"d\")",
		},
		{
			desc: "RowRange closed closed",
			rr:   NewClosedRange("e", "f"),
			str:  "[\"e\",\"f\"]",
		},
		{
			desc: "RowRange open closed",
			rr:   NewOpenClosedRange("g", "h"),
			str:  "(\"g\",\"h\"]",
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
			str:  "(∞,\"c\"]",
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
	client, err := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, disableMetricsConfig, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClientWithConfig failed: %v", err)
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

	client, err := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, disableMetricsConfig, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClientWithConfig failed: %v", err)
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

func TestReadRowsLimit(t *testing.T) {
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

	client, err := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, disableMetricsConfig, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClientWithConfig failed: %v", err)
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

	for _, test := range []struct {
		desc         string
		limit        *int64
		wantRowCount int64
		wantErr      error
	}{
		{
			desc:         "No limit",
			wantRowCount: 3,
		},
		{
			desc:         "Limit less than number of rows in table",
			limit:        ptr(int64(2)),
			wantRowCount: 2,
		},
		{
			desc:         "Limit greater than number of rows in table",
			limit:        ptr(int64(5)),
			wantRowCount: 3,
		},
		{
			desc:    "Negative row limit",
			limit:   ptr(int64(-1)),
			wantErr: errNegativeRowLimit,
		},
	} {
		gotRowCount := int64(0)
		t.Run(test.desc, func(t *testing.T) {
			opts := []ReadOption{}
			if test.limit != nil {
				opts = append(opts, LimitRows(*test.limit))
			}
			if err := table.ReadRows(ctx, InfiniteRange(""), func(r Row) bool {
				gotRowCount++
				return true
			}, opts...); !errors.Is(err, test.wantErr) {
				t.Errorf("ReadRows err got: %v, want: %v", err, test.wantErr)
			}

			if gotRowCount != test.wantRowCount {
				t.Errorf("ReadRows returned %d rows, want %d", gotRowCount, test.wantRowCount)
			}
		})
	}
}

// ptr returns a pointer to its argument.
// It can be used to initialize pointer fields:
func ptr[T any](t T) *T { return &t }

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

func TestMutateRowsWithAggregates_AddToCell(t *testing.T) {
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

	client, err := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, disableMetricsConfig, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClientWithConfig failed: %v", err)
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

func TestMutateRowsWithAggregates_MergeToCell(t *testing.T) {
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

	client, err := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, ClientConfig{MetricsProvider: NoopMetricsProvider{}}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()
	table := client.Open(testEnv.config.Table)

	m := NewMutation()
	m.MergeBytesToCell("f", "q", 0, binary.BigEndian.AppendUint64([]byte{}, 1000))
	err = table.Apply(ctx, "row1", m)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	m = NewMutation()
	m.MergeBytesToCell("f", "q", 0, binary.BigEndian.AppendUint64([]byte{}, 2000))
	err = table.Apply(ctx, "row1", m)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	row, err := table.ReadRow(ctx, "row1")
	if !bytes.Equal(row["f"][0].Value, binary.BigEndian.AppendUint64([]byte{}, 3000)) {
		t.Error()
	}
}

type rowKeyCheckingInterceptor struct {
	grpc.ClientStream
	failRow        string
	failErr        error // error to use while sending failed response for fail row
	requestCounter *int
}

func (i *rowKeyCheckingInterceptor) SendMsg(m interface{}) error {
	*i.requestCounter = *i.requestCounter + 1
	if req, ok := m.(*btpb.MutateRowsRequest); ok {
		for _, entry := range req.Entries {
			if string(entry.RowKey) == i.failRow {
				return i.failErr
			}
		}
	}
	return i.ClientStream.SendMsg(m)
}

func (i *rowKeyCheckingInterceptor) RecvMsg(m interface{}) error {
	return i.ClientStream.RecvMsg(m)
}

// Mutations are broken down into groups of 'maxMutations' and then MutateRowsRequest is sent to Cloud Bigtable Service
// This test validates that even if one of the group receives error, requests are sent for further groups
func TestApplyBulk_MutationsSucceedAfterGroupError(t *testing.T) {
	testEnv, gotErr := NewEmulatedEnv(IntegrationTestConfig{})
	if gotErr != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", gotErr)
	}

	// Add interceptor to fail rows
	failedRow := "row2"
	failErr := status.Error(codes.InvalidArgument, "Invalid row key")
	reqCount := 0
	conn, gotErr := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20)),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			clientStream, err := streamer(ctx, desc, cc, method, opts...)
			return &rowKeyCheckingInterceptor{
				ClientStream:   clientStream,
				failRow:        failedRow,
				requestCounter: &reqCount,
				failErr:        failErr,
			}, err
		}),
	)
	if gotErr != nil {
		t.Fatalf("grpc.Dial failed: %v", gotErr)
	}

	// Create client and table
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminClient, gotErr := NewAdminClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if gotErr != nil {
		t.Fatalf("NewClient failed: %v", gotErr)
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
	client, gotErr := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, disableMetricsConfig, option.WithGRPCConn(conn))
	if gotErr != nil {
		t.Fatalf("NewClientWithConfig failed: %v", gotErr)
	}
	defer client.Close()
	table := client.Open(testEnv.config.Table)

	// Override maxMutations to break mutations into smaller groups
	origMaxMutations := maxMutations
	t.Cleanup(func() {
		maxMutations = origMaxMutations
	})
	maxMutations = 2

	// Create mutations
	m1 := NewMutation()
	m1.AddIntToCell("f", "q", 0, 1000)
	m2 := NewMutation()
	m2.AddIntToCell("f", "q", 0, 2000)

	// Perform ApplyBulk operation and compare errors
	rowKeys := []string{"row1", "row1", failedRow, failedRow, "row3", "row3"}
	var wantErr error
	wantErrs := []error{nil, nil, failErr, failErr, nil, nil}
	gotErrs, gotErr := table.ApplyBulk(ctx, rowKeys, []*Mutation{m1, m2, m1, m2, m1, m2})

	// Assert overall error
	if !equalErrs(gotErr, wantErr) {
		t.Fatalf("ApplyBulk err got: %v, want: %v", gotErr, wantErr)
	}

	// Assert individual muation errors
	if len(gotErrs) != len(wantErrs) {
		t.Fatalf("ApplyBulk errs got: %v, want: %v", gotErrs, wantErrs)
	}
	for i := range gotErrs {
		if !equalErrs(gotErrs[i], wantErrs[i]) {
			t.Errorf("#%d ApplyBulk err got: %v, want: %v", i, gotErrs[i], wantErrs[i])
		}
	}

	// Assert number of requests sent
	wantReqCount := len(rowKeys) / maxMutations
	if reqCount != wantReqCount {
		t.Errorf("Number of requests got: %v, want: %v", reqCount, wantReqCount)
	}

	// Assert individual mutation apply success/failure by reading rows
	gotErr = table.ReadRows(ctx, RowList{"row1", failedRow, "row3"}, func(row Row) bool {
		rowMutated := bytes.Equal(row["f"][0].Value, binary.BigEndian.AppendUint64([]byte{}, 3000))
		if rowMutated && row.Key() == failedRow {
			t.Error("Expected mutation to fail for row " + row.Key())
		}
		if !rowMutated && row.Key() != failedRow {
			t.Error("Expected mutation to succeed for row " + row.Key())
		}
		return true
	})
	if gotErr != nil {
		t.Fatalf("ReadRows failed: %v", gotErr)
	}
}

func TestAnySQLTypeToPbVal(t *testing.T) {
	testTime := time.Now()
	testDate := civil.DateOf(time.Now())

	tests := []struct {
		testName   string
		paramVal   any
		psType     SQLType
		wantPbVal  *btpb.Value
		wantErr    bool
		wantErrMsg string
	}{
		{
			testName: "BytesSQLType success",
			paramVal: []byte("test"),
			psType:   BytesSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_BytesType{
						BytesType: &btpb.Type_Bytes{},
					},
				},
				Kind: &btpb.Value_BytesValue{
					BytesValue: []byte("test"),
				},
			},
		},
		{
			testName: "BytesSQLType nil success",
			psType:   BytesSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_BytesType{
						BytesType: &btpb.Type_Bytes{},
					},
				},
			},
		},
		{
			testName:   "BytesSQLType type mismatch",
			paramVal:   "test",
			psType:     BytesSQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "test", psType: BytesSQLType{}}).Error(),
		},
		{
			testName: "StringSQLType success",
			paramVal: "test",
			psType:   StringSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_StringType{
						StringType: &btpb.Type_String{},
					},
				},
				Kind: &btpb.Value_StringValue{
					StringValue: "test",
				},
			},
		},
		{
			testName: "StringSQLType nil success",
			psType:   StringSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_StringType{
						StringType: &btpb.Type_String{},
					},
				},
			},
		},
		{
			testName:   "StringSQLType type mismatch",
			paramVal:   123,
			psType:     StringSQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: 123, psType: StringSQLType{}}).Error(),
		},
		{
			testName: "Int64SQLType success",
			paramVal: int64(123),
			psType:   Int64SQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_Int64Type{
						Int64Type: &btpb.Type_Int64{},
					},
				},
				Kind: &btpb.Value_IntValue{
					IntValue: int64(123),
				},
			},
		},
		{
			testName: "Int64SQLType nil success",
			psType:   Int64SQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_Int64Type{
						Int64Type: &btpb.Type_Int64{},
					},
				},
			},
		},
		{
			testName:   "Int64SQLType type mismatch",
			paramVal:   "123",
			psType:     Int64SQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "123", psType: Int64SQLType{}}).Error(),
		},
		{
			testName: "Float32SQLType success",
			paramVal: float32(1.23),
			psType:   Float32SQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_Float32Type{
						Float32Type: &btpb.Type_Float32{},
					},
				},
				Kind: &btpb.Value_FloatValue{
					FloatValue: float64(1.23),
				},
			},
		},
		{
			testName: "Float32SQLType nil success",
			psType:   Float32SQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_Float32Type{
						Float32Type: &btpb.Type_Float32{},
					},
				},
			},
		},
		{
			testName:   "Float32SQLType type mismatch - string",
			paramVal:   "1.23",
			psType:     Float32SQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "1.23", psType: Float32SQLType{}}).Error(),
		},
		{
			testName:   "Float32SQLType type mismatch - float64",
			paramVal:   float64(1.23),
			psType:     Float32SQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: float64(1.23), psType: Float32SQLType{}}).Error(),
		},
		{
			testName: "Float64SQLType success",
			paramVal: float64(1.23),
			psType:   Float64SQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_Float64Type{
						Float64Type: &btpb.Type_Float64{},
					},
				},
				Kind: &btpb.Value_FloatValue{
					FloatValue: float64(1.23),
				},
			},
		},
		{
			testName: "Float64SQLType nil success",
			psType:   Float64SQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_Float64Type{
						Float64Type: &btpb.Type_Float64{},
					},
				},
			},
		},
		{
			testName:   "Float64SQLType type mismatch - string",
			paramVal:   "1.23",
			psType:     Float64SQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "1.23", psType: Float64SQLType{}}).Error(),
		},
		{
			testName:   "Float64SQLType type mismatch - float32",
			paramVal:   float32(1.23),
			psType:     Float64SQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: float32(1.23), psType: Float64SQLType{}}).Error(),
		},
		{
			testName: "BoolSQLType success",
			paramVal: true,
			psType:   BoolSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_BoolType{
						BoolType: &btpb.Type_Bool{},
					},
				},
				Kind: &btpb.Value_BoolValue{
					BoolValue: true,
				},
			},
		},
		{
			testName: "BoolSQLType nil success",
			psType:   BoolSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_BoolType{
						BoolType: &btpb.Type_Bool{},
					},
				},
			},
		},
		{
			testName:   "BoolSQLType type mismatch",
			paramVal:   "true",
			psType:     BoolSQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "true", psType: BoolSQLType{}}).Error(),
		},
		{
			testName: "TimestampSQLType success",
			paramVal: testTime,
			psType:   TimestampSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_TimestampType{
						TimestampType: &btpb.Type_Timestamp{},
					},
				},
				Kind: &btpb.Value_TimestampValue{
					TimestampValue: timestamppb.New(testTime),
				},
			},
		},
		{
			testName: "TimestampSQLType nil success",
			psType:   TimestampSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_TimestampType{
						TimestampType: &btpb.Type_Timestamp{},
					},
				},
			},
		},
		{
			testName:   "TimestampSQLType type mismatch",
			paramVal:   "2024-01-01",
			psType:     TimestampSQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "2024-01-01", psType: TimestampSQLType{}}).Error(),
		},
		{
			testName: "DateSQLType success",
			paramVal: testDate,
			psType:   DateSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_DateType{
						DateType: &btpb.Type_Date{},
					},
				},
				Kind: &btpb.Value_DateValue{
					DateValue: &date.Date{Year: int32(testDate.Year), Month: int32(testDate.Month), Day: int32(testDate.Day)},
				},
			},
		},
		{
			testName: "DateSQLType nil success",
			psType:   DateSQLType{},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_DateType{
						DateType: &btpb.Type_Date{},
					},
				},
			},
		},
		{
			testName:   "DateSQLType type mismatch",
			paramVal:   "2024-01-01",
			psType:     DateSQLType{},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "2024-01-01", psType: DateSQLType{}}).Error(),
		},
		{
			testName: "ArraySQLType success concrete type",
			paramVal: []int64{1, 2, 3},
			psType:   ArraySQLType{ElemType: Int64SQLType{}},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_ArrayType{
						ArrayType: &btpb.Type_Array{
							ElementType: &btpb.Type{
								Kind: &btpb.Type_Int64Type{
									Int64Type: &btpb.Type_Int64{},
								},
							},
						},
					},
				},
				Kind: &btpb.Value_ArrayValue{
					ArrayValue: &btpb.ArrayValue{
						Values: []*btpb.Value{
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(1),
								},
							},
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(2),
								},
							},
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(3),
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "ArraySQLType success any type with nil",
			paramVal: []any{1, 2, 3, nil},
			psType:   ArraySQLType{ElemType: Int64SQLType{}},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_ArrayType{
						ArrayType: &btpb.Type_Array{
							ElementType: &btpb.Type{
								Kind: &btpb.Type_Int64Type{
									Int64Type: &btpb.Type_Int64{},
								},
							},
						},
					},
				},
				Kind: &btpb.Value_ArrayValue{
					ArrayValue: &btpb.ArrayValue{
						Values: []*btpb.Value{
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(1),
								},
							},
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(2),
								},
							},
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(3),
								},
							},
							{},
						},
					},
				},
			},
		},
		{
			testName: "ArraySQLType success int32 in int64",
			paramVal: []int32{1, 2, 3},
			psType:   ArraySQLType{ElemType: Int64SQLType{}},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_ArrayType{
						ArrayType: &btpb.Type_Array{
							ElementType: &btpb.Type{
								Kind: &btpb.Type_Int64Type{
									Int64Type: &btpb.Type_Int64{},
								},
							},
						},
					},
				},
				Kind: &btpb.Value_ArrayValue{
					ArrayValue: &btpb.ArrayValue{
						Values: []*btpb.Value{
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(1),
								},
							},
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(2),
								},
							},
							{
								Kind: &btpb.Value_IntValue{
									IntValue: int64(3),
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "ArraySQLType nil success",
			psType:   ArraySQLType{ElemType: Int64SQLType{}},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_ArrayType{
						ArrayType: &btpb.Type_Array{
							ElementType: &btpb.Type{
								Kind: &btpb.Type_Int64Type{
									Int64Type: &btpb.Type_Int64{},
								},
							},
						},
					},
				},
			},
		},
		{
			testName: "ArraySQLType empty array success",
			paramVal: []int64{},
			psType:   ArraySQLType{ElemType: Int64SQLType{}},
			wantPbVal: &btpb.Value{
				Type: &btpb.Type{
					Kind: &btpb.Type_ArrayType{
						ArrayType: &btpb.Type_Array{
							ElementType: &btpb.Type{
								Kind: &btpb.Type_Int64Type{
									Int64Type: &btpb.Type_Int64{},
								},
							},
						},
					},
				},
				Kind: &btpb.Value_ArrayValue{
					ArrayValue: &btpb.ArrayValue{},
				},
			},
		},
		{
			testName:   "ArraySQLType type mismatch",
			paramVal:   "not an array",
			psType:     ArraySQLType{ElemType: Int64SQLType{}},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "not an array", psType: ArraySQLType{ElemType: Int64SQLType{}}}).Error(),
		},
		{
			testName:   "ArraySQLType element type mismatch",
			paramVal:   []any{int64(1), "not an int", int64(3)},
			psType:     ArraySQLType{ElemType: Int64SQLType{}},
			wantErr:    true,
			wantErrMsg: ptr(errTypeMismatch{value: "not an int", psType: Int64SQLType{}}).Error(),
		},
		{
			testName:   "ArraySQLType unsupported ElemType",
			paramVal:   []int64{1, 2, 3},
			psType:     ArraySQLType{ElemType: ArraySQLType{ElemType: Int64SQLType{}}},
			wantErr:    true,
			wantErrMsg: "bigtable: unsupported ElemType: bigtable.ArraySQLType",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			got, err := anySQLTypeToPbVal(tt.paramVal, tt.psType)
			if (err != nil) != tt.wantErr {
				t.Errorf("error got: %v, want: nil", err)
				return
			}
			if tt.wantErr {
				if err != nil && err.Error() != tt.wantErrMsg {
					t.Errorf("error got: %v, want: %v", err, tt.wantErrMsg)
				}
				return
			}
			if !cmp.Equal(got, tt.wantPbVal, cmpOptionsBtpbValue()...) {
				t.Errorf("SQLType value got: %+v, want: %+v, diff: %v", got, tt.wantPbVal, cmp.Diff(got, tt.wantPbVal, cmpOptionsBtpbValue()...))
			}
		})
	}
}

func cmpOptionsBtpbValue() []cmp.Option {
	return []cmp.Option{cmpopts.IgnoreUnexported(btpb.Value{}, btpb.Type{},
		btpb.Type_BytesType{}, btpb.Type_Bytes{},
		btpb.Type_StringType{}, btpb.Type_String{},
		btpb.Type_Int64Type{}, btpb.Type_Int64{},
		btpb.Type_Float32Type{}, btpb.Type_Float32{},
		btpb.Type_Float64Type{}, btpb.Type_Float64{},
		btpb.Type_BoolType{}, btpb.Type_Bool{},
		btpb.Type_TimestampType{}, btpb.Type_Timestamp{},
		btpb.Type_DateType{}, btpb.Type_Date{},
		btpb.Type_ArrayType{}, btpb.Type_Array{},
		btpb.Value_BytesValue{},
		btpb.Value_StringValue{},
		btpb.Value_IntValue{},
		btpb.Value_FloatValue{},
		btpb.Value_BoolValue{},
		btpb.Value_TimestampValue{}, timestamppb.Timestamp{},
		btpb.Value_DateValue{}, date.Date{},
		btpb.Value_ArrayValue{}, btpb.ArrayValue{}),
		cmpopts.IgnoreFields(btpb.Value_FloatValue{}, "FloatValue")}
}

func TestPreparedStatementBind(t *testing.T) {
	tests := []struct {
		testName   string
		query      string
		paramTypes map[string]SQLType
		values     map[string]any
		wantErr    bool
		wantErrMsg string
	}{
		{
			testName:   "no parameter error",
			paramTypes: map[string]SQLType{},
			values:     map[string]any{"param1": "value1"},
			wantErr:    true,
			wantErrMsg: "bigtable: no parameter with name param1 in prepared statement",
		},
		{
			testName:   "not bound error - single missing",
			paramTypes: map[string]SQLType{"param1": StringSQLType{}, "param2": StringSQLType{}},
			values:     map[string]any{"param1": "value1"},
			wantErr:    true,
			wantErrMsg: "bigtable: parameter param2 not bound in prepared statement",
		},
		{
			testName:   "not bound error - all missing",
			paramTypes: map[string]SQLType{"param1": StringSQLType{}, "param2": StringSQLType{}},
			values:     nil,
			wantErr:    true,
			wantErrMsg: "not bound in prepared statement",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			ps := PreparedStatement{
				paramTypes: tt.paramTypes,
			}

			_, err := ps.Bind(tt.values)
			if err == nil && tt.wantErr {
				t.Fatalf("Bind: err got: nil, want: %v", tt.wantErrMsg)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Fatalf("Bind: err got: %v, want: %v", err, tt.wantErrMsg)
			}
		})
	}
}

func TestExecuteQuery(t *testing.T) {
	// start emulated server
	var gotPrepReqCount int
	var mockPrepQueryResps []*btpb.PrepareQueryResponse
	var mockPrepQueryErrs []error

	var gotRecvMsgCount int
	var mockRecvMsgResps []*btpb.ExecuteQueryResponse_Results
	var mockRecvMsgBlockedTimes []time.Duration
	var mockRecvMsgErrs []error
	var gotSendMsgReqs []*btpb.ExecuteQueryRequest

	testEnv, gotErr := NewEmulatedEnv(IntegrationTestConfig{})
	if gotErr != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", gotErr)
	}
	conn, gotErr := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20)),
		grpc.WithStreamInterceptor(
			newStreamClientInterceptor(&gotRecvMsgCount, &gotSendMsgReqs, &mockRecvMsgResps, &mockRecvMsgBlockedTimes, &mockRecvMsgErrs)),
		grpc.WithUnaryInterceptor(
			newUnaryClientInterceptor(&gotPrepReqCount, &mockPrepQueryResps, &mockPrepQueryErrs)),
	)
	if gotErr != nil {
		t.Fatalf("grpc.Dial failed: %v", gotErr)
	}

	// Create client
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	client, gotErr := NewClientWithConfig(ctx, testEnv.config.Project, testEnv.config.Instance, disableMetricsConfig, option.WithGRPCConn(conn))
	if gotErr != nil {
		t.Fatalf("NewClientWithConfig failed: %v", gotErr)
	}
	defer client.Close()

	stPcf, _ := status.New(codes.InvalidArgument, "invalid argument").WithDetails(&errdetails.PreconditionFailure{
		Violations: []*errdetails.PreconditionFailure_Violation{
			{
				Type:        queryExpiredViolationType,
				Description: "The prepared query has expired. Please re-issue the ExecuteQuery with a valid prepared query.",
			},
		},
	})
	aePcf, _ := apierror.FromError(stPcf.Err())
	preparedQuery1 := "first mock prepared query"
	preparedQuery2 := "second mock prepared query"
	for _, tc := range []struct {
		desc                    string
		mockPrepQueryResps      []*btpb.PrepareQueryResponse
		mockPrepQueryErrs       []error
		mockRecvMsgResps        []*btpb.ExecuteQueryResponse_Results
		mockRecvMsgBlockedTimes []time.Duration
		mockRecvMsgErrs         []error
		wantExecReqPrepQuerys   [][]byte
		wantResultRowValues     [][]*btpb.Value
		wantExecErr             error
	}{
		{
			desc: "success",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress, colFamInfo), // PrepareStatement
			},
			mockPrepQueryErrs: []error{
				nil,
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				mockExecuteQueryResponseWithPartialBatchDataFirstHalf(true, colFamAddress),
				mockExecuteQueryResponseWithPartialBatchDataSecondHalf(false, colFamInfo),
				mockExecuteQueryResponseWithResumeToken(),
				{},
			},
			mockRecvMsgErrs: []error{
				nil,
				nil,
				nil,
				io.EOF,
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1),
			},
			wantResultRowValues: [][]*btpb.Value{mockProtoRowValues(colFamAddress, colFamInfo)},
		},
		{
			desc: "server stream ended without ResumeToken",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress), // PrepareStatement
			},
			mockPrepQueryErrs: []error{
				nil,
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				mockExecuteQueryResponseWithBatchData(true, colFamAddress),
				{},
			},
			mockRecvMsgErrs: []error{
				nil,
				io.EOF,
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1),
			},
			wantExecErr: errors.New("bigtable: server stream ended without sending a resume token"),
		},
		{
			desc: "retry on expired query FailedPrecondition error",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress), // PrepareStatement
				mockPrepareQueryResponse(preparedQuery2, colFamAddress), // Execute
			},
			mockPrepQueryErrs: []error{
				nil,
				nil,
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				{},
				mockExecuteQueryResponseWithBatchData(true, colFamAddress),
				mockExecuteQueryResponseWithResumeToken(),
				{},
			},
			mockRecvMsgErrs: []error{
				aePcf,
				nil,
				nil,
				io.EOF,
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1),
				[]byte(preparedQuery2),
			},
			wantResultRowValues: [][]*btpb.Value{mockProtoRowValues(colFamAddress)},
		},
		{
			desc: "transient error after receiving first resume token should not refresh query",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress), // PrepareStatement
			},
			mockPrepQueryErrs: []error{
				nil,
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				mockExecuteQueryResponseWithBatchData(true, colFamAddress),
				mockExecuteQueryResponseWithResumeToken(),
				{},
				{},
			},
			mockRecvMsgErrs: []error{
				nil,
				nil,
				status.Error(codes.Unavailable, "transient error"),
				io.EOF,
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1),
				[]byte(preparedQuery1),
			},
			wantResultRowValues: [][]*btpb.Value{mockProtoRowValues(colFamAddress)},
		},
		{
			desc: "retry on time-based expired query",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress), // PrepareStatement
				mockPrepareQueryResponse(preparedQuery1, colFamAddress), // From Execute, because expired query
			},
			mockPrepQueryErrs: []error{
				nil,
				nil,
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				mockExecuteQueryResponseWithBatchData(true, colFamAddress),
				{},
				mockExecuteQueryResponseWithResumeToken(),
				{},
			},
			mockRecvMsgBlockedTimes: []time.Duration{
				0,
				testPreparedQueryTTL + 2*time.Second,
				0,
				0,
			},
			mockRecvMsgErrs: []error{
				nil,
				status.Error(codes.DeadlineExceeded, "context deadline exceeded"), // retryable
				nil,
				io.EOF,
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1),
				[]byte(preparedQuery1),
			},
			wantResultRowValues: [][]*btpb.Value{mockProtoRowValues(colFamAddress)},
		},
		{
			desc: "retryable error from PrepareQuery should retry PrepareQuery and Execute",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress), // PrepareStatement
				{}, // Execute
				mockPrepareQueryResponse(preparedQuery1, colFamAddress),
			},
			mockPrepQueryErrs: []error{
				nil,
				status.Error(codes.DeadlineExceeded, "context deadline exceeded"), // retryable
				nil,
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				{},
				mockExecuteQueryResponseWithBatchData(true, colFamAddress),
				mockExecuteQueryResponseWithResumeToken(),
				{},
			},
			mockRecvMsgErrs: []error{
				aePcf,
				nil,
				nil,
				io.EOF,
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1),
				[]byte(preparedQuery1),
			},
			wantResultRowValues: [][]*btpb.Value{mockProtoRowValues(colFamAddress)},
		},
		{
			/*
				1. PrepareQuery
				2. ExecuteQuery
				3. RecvMsg - gets first batch of data with reset true
				4. RecvMsg - gets second batch of data with reset false
				5. RecvMsg - gets Unavailable error
				6. ExecuteQuery
				7. RecvMsg - gets query expired error
				8. PrepareQuery - receives changed metadata
				9. ExecuteQuery
				10. RecvMsg - gets first batch of new data with reset true
				11. RecvMsg - gets second batch with resume token
				12. RecvMsg - gets EOF error
			*/
			desc: "batch should be discarded if metadata changed and reset true",
			mockPrepQueryResps: []*btpb.PrepareQueryResponse{
				mockPrepareQueryResponse(preparedQuery1, colFamAddress),             // Step 1
				mockPrepareQueryResponse(preparedQuery2, colFamAddress, colFamInfo), // Step 8
			},
			mockPrepQueryErrs: []error{
				nil, // Step 1
				nil, // Step 8
			},
			mockRecvMsgResps: []*btpb.ExecuteQueryResponse_Results{
				mockExecuteQueryResponseWithBatchData(true, colFamAddress),  // Step 3
				mockExecuteQueryResponseWithBatchData(false, colFamAddress), // Step 4
				{}, // Step 5
				{}, // Step 7
				mockExecuteQueryResponseWithBatchData(true, colFamAddress, colFamInfo), // Step 10
				mockExecuteQueryResponseWithResumeToken(),                              // Step 11
				{}, // Step 12
			},
			mockRecvMsgErrs: []error{
				nil, // Step 3
				nil, // Step 4
				status.Error(codes.Unavailable, "mock unavailable error"), // retryable error Step 5
				aePcf,  // retryable error Step 7
				nil,    // Step 10
				nil,    // Step 11
				io.EOF, // Step 12
			},
			wantExecReqPrepQuerys: [][]byte{
				[]byte(preparedQuery1), // Step 2
				[]byte(preparedQuery1), // Step 6
				[]byte(preparedQuery2), // Step 9
			},
			wantResultRowValues: [][]*btpb.Value{mockProtoRowValues(colFamAddress, colFamInfo)},
		},
	} {
		mockPrepQueryResps = tc.mockPrepQueryResps
		mockPrepQueryErrs = tc.mockPrepQueryErrs
		mockRecvMsgResps = tc.mockRecvMsgResps
		mockRecvMsgErrs = tc.mockRecvMsgErrs
		mockRecvMsgBlockedTimes = tc.mockRecvMsgBlockedTimes

		// Reset vars for the test
		gotPrepReqCount = 0
		gotRecvMsgCount = 0
		gotSendMsgReqs = []*btpb.ExecuteQueryRequest{}

		t.Run(tc.desc, func(t *testing.T) {
			// Prepare query
			ps, err := client.PrepareStatement(ctx, "SELECT * FROM users;", nil)
			if err != nil {
				t.Fatalf("PrepareStatement: %v", err)
			}
			bs, err := ps.Bind(nil)
			if err != nil {
				t.Fatalf("Bind: %v", err)
			}

			// Execute query

			gotRowCount := 0
			err = bs.Execute(ctx, func(rr ResultRow) bool {
				vals := rr.values
				if gotRowCount > len(tc.wantResultRowValues) ||
					!cmp.Equal(vals, tc.wantResultRowValues[gotRowCount], cmpOptionsBtpbValue()...) {
					t.Errorf("#%d ResultRow.values: got: %+v, want: %+v, diff: %+v", gotRowCount, vals, tc.wantResultRowValues[gotRowCount],
						cmp.Diff(vals, tc.wantResultRowValues[gotRowCount], cmpOptionsBtpbValue()...))
					return false
				}
				return true
			})

			if tc.wantExecErr != nil && err != nil && err.Error() != tc.wantExecErr.Error() {
				t.Fatalf("Execute: err: got: %v, want: %v", err, tc.wantExecErr)
			} else if (err != nil && tc.wantExecErr == nil) || (err == nil && tc.wantExecErr != nil) {
				t.Fatalf("Execute: err got: %v, want: %v", err, tc.wantExecErr)
			}

			if gotPrepReqCount != len(tc.mockPrepQueryResps) {
				t.Fatalf("PrepareQuery request count: got: %v, want: %v", gotPrepReqCount, len(tc.mockPrepQueryResps))
			}
			if gotRecvMsgCount != len(tc.mockRecvMsgResps) {
				t.Fatalf("RecvMsg request count: got: %v, want: %v", gotRecvMsgCount, len(tc.mockRecvMsgResps))
			}

			if len(tc.wantExecReqPrepQuerys) != len(gotSendMsgReqs) {
				t.Fatalf("ExecuteQuery request count: got: %v, want: %v", len(gotSendMsgReqs), len(tc.wantExecReqPrepQuerys))
			}

			for i, wantPrepQueryInExecReq := range tc.wantExecReqPrepQuerys {
				if string(gotSendMsgReqs[i].PreparedQuery) != string(wantPrepQueryInExecReq) {
					t.Fatalf("%v: PreparedQuery in ExecuteQuery request: got: %v, want: %v",
						i,
						string(gotSendMsgReqs[i].PreparedQuery), string(wantPrepQueryInExecReq))
				}
			}
		})
	}
}

const testPreparedQueryTTL = 10 * time.Second

func mockPrepareQueryResponse(preparedQuery string, colFams ...string) *btpb.PrepareQueryResponse {
	bytesType := &btpb.Type{
		Kind: &btpb.Type_BytesType{
			BytesType: &btpb.Type_Bytes{},
		},
	}

	columns := []*btpb.ColumnMetadata{
		{
			Name: "_key",
			Type: bytesType,
		},
	}

	for _, cf := range colFams {
		columns = append(columns, &btpb.ColumnMetadata{
			Name: cf,
			Type: &btpb.Type{
				Kind: &btpb.Type_MapType{
					MapType: &btpb.Type_Map{
						KeyType:   bytesType,
						ValueType: bytesType,
					},
				},
			},
		})
	}
	return &btpb.PrepareQueryResponse{
		PreparedQuery: []byte(preparedQuery),
		Metadata: &btpb.ResultSetMetadata{
			Schema: &btpb.ResultSetMetadata_ProtoSchema{
				ProtoSchema: &btpb.ProtoSchema{
					Columns: columns,
				},
			},
		},
	}
}

const colFamAddress = "address"
const colFamInfo = "info"

var cfToValues = map[string][]*btpb.Value{
	colFamAddress: {
		{
			Kind: &btpb.Value_ArrayValue{
				ArrayValue: &btpb.ArrayValue{
					Values: []*btpb.Value{
						{
							Kind: &btpb.Value_BytesValue{
								BytesValue: []byte("city"),
							},
						},
						{
							Kind: &btpb.Value_BytesValue{
								BytesValue: []byte("San Francisco"),
							},
						},
					},
				},
			},
		},
		{
			Kind: &btpb.Value_ArrayValue{
				ArrayValue: &btpb.ArrayValue{
					Values: []*btpb.Value{
						{
							Kind: &btpb.Value_BytesValue{
								BytesValue: []byte("state"),
							},
						},
						{
							Kind: &btpb.Value_BytesValue{
								BytesValue: []byte("CA"),
							},
						},
					},
				},
			},
		},
	},
	colFamInfo: {
		{
			Kind: &btpb.Value_ArrayValue{
				ArrayValue: &btpb.ArrayValue{
					Values: []*btpb.Value{
						{
							Kind: &btpb.Value_BytesValue{
								BytesValue: []byte("greeting"),
							},
						},
						{
							Kind: &btpb.Value_BytesValue{
								BytesValue: []byte("Hey there"),
							},
						},
					},
				},
			},
		},
	},
}

func mockProtoRowValues(colFams ...string) []*btpb.Value {
	values := []*btpb.Value{
		{
			Kind: &btpb.Value_BytesValue{
				BytesValue: []byte("row-01"),
			},
		},
	}
	values = append(values, mockProtoRowValuesWithoutKey(colFams...)...)
	return values
}

func mockProtoRowValuesWithoutKey(colFams ...string) []*btpb.Value {
	values := []*btpb.Value{}
	for _, cf := range colFams {
		values = append(values, &btpb.Value{
			Kind: &btpb.Value_ArrayValue{
				ArrayValue: &btpb.ArrayValue{
					Values: cfToValues[cf],
				},
			},
		})
	}
	return values
}

func mockExecuteQueryResponseWithBatchData(reset bool, colFams ...string) *btpb.ExecuteQueryResponse_Results {
	protoRows := &btpb.ProtoRows{
		Values: mockProtoRowValues(colFams...),
	}
	marshalled, _ := proto.Marshal(protoRows)
	checksum := crc32.Checksum(marshalled, crc32cTable)
	return &btpb.ExecuteQueryResponse_Results{
		Results: &btpb.PartialResultSet{
			PartialRows: &btpb.PartialResultSet_ProtoRowsBatch{
				ProtoRowsBatch: &btpb.ProtoRowsBatch{
					BatchData: marshalled,
				},
			},
			BatchChecksum: &checksum,
			Reset_:        reset,
		},
	}
}

func mockExecuteQueryResponseWithPartialBatchDataFirstHalf(reset bool, colFams ...string) *btpb.ExecuteQueryResponse_Results {
	protoRows := &btpb.ProtoRows{
		Values: mockProtoRowValues(colFams...),
	}
	marshalled, _ := proto.Marshal(protoRows)
	checksum := crc32.Checksum(marshalled, crc32cTable)
	return &btpb.ExecuteQueryResponse_Results{
		Results: &btpb.PartialResultSet{
			PartialRows: &btpb.PartialResultSet_ProtoRowsBatch{
				ProtoRowsBatch: &btpb.ProtoRowsBatch{
					BatchData: marshalled,
				},
			},
			BatchChecksum: &checksum,
			Reset_:        reset,
		},
	}
}

func mockExecuteQueryResponseWithPartialBatchDataSecondHalf(reset bool, colFams ...string) *btpb.ExecuteQueryResponse_Results {
	protoRows := &btpb.ProtoRows{
		Values: mockProtoRowValuesWithoutKey(colFams...),
	}
	marshalled, _ := proto.Marshal(protoRows)
	checksum := crc32.Checksum(marshalled, crc32cTable)
	return &btpb.ExecuteQueryResponse_Results{
		Results: &btpb.PartialResultSet{
			PartialRows: &btpb.PartialResultSet_ProtoRowsBatch{
				ProtoRowsBatch: &btpb.ProtoRowsBatch{
					BatchData: marshalled,
				},
			},
			BatchChecksum: &checksum,
			Reset_:        reset,
		},
	}
}

func mockExecuteQueryResponseWithResumeToken() *btpb.ExecuteQueryResponse_Results {
	return &btpb.ExecuteQueryResponse_Results{
		Results: &btpb.PartialResultSet{
			ResumeToken: []byte("resume-token"),
		},
	}
}

// wrappedClientStream wraps around the embedded grpc.ClientStream, and intercepts the RecvMsg and
// SendMsg method call.
type wrappedClientStream struct {
	recvMsgCount *int
	reqPtrs      *[]*btpb.ExecuteQueryRequest
	respPtrs     *[]*btpb.ExecuteQueryResponse_Results
	blockedTimes *[]time.Duration
	errPtrs      *[]error
	grpc.ClientStream
}

func (w *wrappedClientStream) RecvMsg(m any) error {
	defer func() { *w.recvMsgCount++ }()
	err := w.ClientStream.RecvMsg(m)
	resp, ok := m.(*btpb.ExecuteQueryResponse)
	if !ok {
		return err
	}

	if *w.blockedTimes != nil {
		sleeps := *w.blockedTimes
		time.Sleep(sleeps[*w.recvMsgCount])
	}
	resps := *w.respPtrs
	errs := *w.errPtrs
	resp.Response = resps[*w.recvMsgCount]
	return errs[*w.recvMsgCount]
}

func (w *wrappedClientStream) SendMsg(m any) error {
	execReq, _ := m.(*btpb.ExecuteQueryRequest)
	*w.reqPtrs = append(*w.reqPtrs, execReq)
	return w.ClientStream.SendMsg(m)
}

func newWrappedClientStream(s grpc.ClientStream, recvMsgCount *int,
	reqPtrs *[]*btpb.ExecuteQueryRequest, respPtrs *[]*btpb.ExecuteQueryResponse_Results, blockedTimes *[]time.Duration,
	errPtrs *[]error) grpc.ClientStream {
	return &wrappedClientStream{
		ClientStream: s,
		recvMsgCount: recvMsgCount,
		reqPtrs:      reqPtrs,
		respPtrs:     respPtrs,
		errPtrs:      errPtrs,
		blockedTimes: blockedTimes,
	}
}

func newStreamClientInterceptor(recvMsgCount *int, reqPtrs *[]*btpb.ExecuteQueryRequest, respPtrs *[]*btpb.ExecuteQueryResponse_Results,
	blockedTimes *[]time.Duration, errPtrs *[]error) func(
	ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
	streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
		streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		s, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}
		return newWrappedClientStream(s, recvMsgCount, reqPtrs, respPtrs, blockedTimes, errPtrs), nil
	}
}

func newUnaryClientInterceptor(prepReqCount *int, respPtrs *[]*btpb.PrepareQueryResponse, errPtrs *[]error) func(
	ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		defer func() { *prepReqCount++ }()
		_, isPrepReq := req.(*btpb.PrepareQueryRequest)
		if !isPrepReq || (err != nil && !strings.Contains(err.Error(), emulatorUnsupported)) {
			return err
		}

		errs := *errPtrs
		currErr := errs[*prepReqCount]
		if currErr != nil {
			return currErr
		}

		resps := *respPtrs
		pqr, _ := reply.(*btpb.PrepareQueryResponse)
		pqr.PreparedQuery = resps[*prepReqCount].PreparedQuery
		pqr.ValidUntil = timestamppb.New(time.Now().Add(testPreparedQueryTTL))
		pqr.Metadata = resps[*prepReqCount].Metadata
		return nil
	}
}
