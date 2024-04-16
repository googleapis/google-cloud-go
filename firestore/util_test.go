// Copyright 2017 Google LLC
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

package firestore

import (
	"context"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	aTime       = time.Date(2017, 1, 26, 0, 0, 0, 0, time.UTC)
	aTime2      = time.Date(2017, 2, 5, 0, 0, 0, 0, time.UTC)
	aTime3      = time.Date(2017, 3, 20, 0, 0, 0, 0, time.UTC)
	aTimestamp  = mustTimestampProto(aTime)
	aTimestamp2 = mustTimestampProto(aTime2)
	aTimestamp3 = mustTimestampProto(aTime3)
)

func mustTimestampProto(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

var cmpOpts = []cmp.Option{
	cmp.AllowUnexported(DocumentSnapshot{},
		Query{}, OrFilter{}, AndFilter{}, PropertyPathFilter{}, PropertyFilter{}, order{}, fpv{}, DocumentRef{}, CollectionRef{}, Query{}),
	cmpopts.IgnoreTypes(Client{}, &Client{}),
	cmp.Comparer(func(*readSettings, *readSettings) bool {
		return true // Don't try to compare two readSettings pointer types
	}),
}

// testEqual implements equality for Firestore tests.
func testEqual(a, b interface{}) bool {
	return testutil.Equal(a, b, cmpOpts...)
}

func testDiff(a, b interface{}) string {
	return testutil.Diff(a, b, cmpOpts...)
}

func TestTestEqual(t *testing.T) {
	for _, test := range []struct {
		a, b interface{}
		want bool
	}{
		{nil, nil, true},
		{([]int)(nil), nil, false},
		{nil, ([]int)(nil), false},
		{([]int)(nil), ([]int)(nil), true},
	} {
		if got := testEqual(test.a, test.b); got != test.want {
			t.Errorf("testEqual(%#v, %#v) == %t, want %t", test.a, test.b, got, test.want)
		}
	}
}

func newMock(t *testing.T) (_ *Client, _ *mockServer, _ func()) {
	srv, cleanup, err := newMockServer()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(context.Background(), "projectID", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv, func() {
		client.Close()
		conn.Close()
		cleanup()
	}
}

func intval(i int) *pb.Value {
	return int64val(int64(i))
}

func int64val(i int64) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_IntegerValue{i}}
}

func boolval(b bool) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_BooleanValue{b}}
}

func floatval(f float64) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_DoubleValue{f}}
}

func strval(s string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_StringValue{s}}
}

func bytesval(b []byte) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_BytesValue{b}}
}

func tsval(t time.Time) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_TimestampValue{TimestampValue: timestamppb.New(t)}}
}

func geoval(ll *latlng.LatLng) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_GeoPointValue{ll}}
}

func arrayval(s ...*pb.Value) *pb.Value {
	if s == nil {
		s = []*pb.Value{}
	}
	return &pb.Value{ValueType: &pb.Value_ArrayValue{&pb.ArrayValue{Values: s}}}
}

func mapval(m map[string]*pb.Value) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{&pb.MapValue{Fields: m}}}
}

func refval(path string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_ReferenceValue{path}}
}
