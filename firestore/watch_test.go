// Copyright 2018 Google Inc. All Rights Reserved.
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
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/btree"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWatchRecv(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	db := defaultBackoff
	defaultBackoff = gax.Backoff{Initial: 1, Max: 1, Multiplier: 1}
	defer func() { defaultBackoff = db }()

	ws := newWatchStream(ctx, c, &pb.Target{})
	request := &pb.ListenRequest{
		Database:     "projects/projectID/databases/(default)",
		TargetChange: &pb.ListenRequest_AddTarget{&pb.Target{}},
	}
	response := &pb.ListenResponse{ResponseType: &pb.ListenResponse_DocumentChange{&pb.DocumentChange{}}}
	// Stream should retry on non-permanent errors, returning only the responses.
	srv.addRPC(request, []interface{}{response, status.Error(codes.Unknown, "")})
	srv.addRPC(request, []interface{}{response}) // stream will return io.EOF
	srv.addRPC(request, []interface{}{response, status.Error(codes.DeadlineExceeded, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.ResourceExhausted, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.Internal, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.Unavailable, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.Unauthenticated, "")})
	srv.addRPC(request, []interface{}{response})
	for i := 0; i < 4; i++ {
		res, err := ws.recv()
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(res, response) {
			t.Fatalf("got %v, want %v", res, response)
		}
	}

	// Stream should not retry on a permanent error.
	srv.addRPC(request, []interface{}{status.Error(codes.AlreadyExists, "")})
	_, err := ws.recv()
	if got, want := status.Code(err), codes.AlreadyExists; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestComputeSnapshot(t *testing.T) {
	c := &Client{
		projectID:  "projID",
		databaseID: "(database)",
	}
	tm := time.Now()
	i := 0
	doc := func(path, value string) *DocumentSnapshot {
		i++
		return &DocumentSnapshot{
			Ref:        c.Doc(path),
			proto:      &pb.Document{Fields: map[string]*pb.Value{"foo": strval(value)}},
			UpdateTime: tm.Add(time.Duration(i) * time.Second), // need unique time for updates
		}
	}
	val := func(d *DocumentSnapshot) string { return d.proto.Fields["foo"].GetStringValue() }
	less := func(a, b interface{}) bool { return val(a.(*DocumentSnapshot)) < val(b.(*DocumentSnapshot)) }

	type dmap map[string]*DocumentSnapshot

	ds1 := doc("C/d1", "a")
	ds2 := doc("C/d2", "b")
	ds2c := doc("C/d2", "c")
	docTree := btree.New(4, less)
	docMap := dmap{}
	// The following test cases are not independent; each builds on the output of the previous.
	for _, test := range []struct {
		desc      string
		changeMap dmap
		want      []*DocumentSnapshot
	}{
		{
			"no changes",
			nil,
			nil,
		},
		{
			"add a doc",
			dmap{ds1.Ref.Path: ds1},
			[]*DocumentSnapshot{ds1},
		},
		{
			"add, remove",
			dmap{ds1.Ref.Path: nil, ds2.Ref.Path: ds2},
			[]*DocumentSnapshot{ds2},
		},
		{
			"add back, modify",
			dmap{ds1.Ref.Path: ds1, ds2c.Ref.Path: ds2c},
			[]*DocumentSnapshot{ds1, ds2c},
		},
	} {
		docTree = computeSnapshot(docTree, docMap, test.changeMap)
		got := treeDocs(docTree)
		if diff := testDiff(got, test.want); diff != "" {
			t.Fatalf("%s: %s", test.desc, diff)
		}
		mgot := mapDocs(docMap, less)
		if diff := testDiff(got, mgot); diff != "" {
			t.Fatalf("%s: docTree and docMap disagree: %s", test.desc, diff)
		}
	}

	// Verify that if there are no changes, the returned docTree is identical to the first arg.
	// docTree already has ds2c.
	got := computeSnapshot(docTree, docMap, dmap{ds2c.Ref.Path: ds2c})
	if got != docTree {
		t.Error("returned docTree != arg docTree")
	}
}

func treeDocs(bt *btree.BTree) []*DocumentSnapshot {
	var ds []*DocumentSnapshot
	it := bt.BeforeIndex(0)
	for it.Next() {
		ds = append(ds, it.Key.(*DocumentSnapshot))
	}
	return ds
}

func mapDocs(m map[string]*DocumentSnapshot, less func(a, b interface{}) bool) []*DocumentSnapshot {
	var ds []*DocumentSnapshot
	for _, d := range m {
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool { return less(ds[i], ds[j]) })
	return ds
}

func TestWatchStream(t *testing.T) {
	// Preliminary, very basic tests. Will expand and turn into cross-language tests
	// later.
	ctx := context.Background()
	c, srv := newMock(t)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	baseTime := time.Now()
	doc := func(path string, value int, tm time.Time) *DocumentSnapshot {
		ref := c.Doc(path)
		ts, err := ptypes.TimestampProto(tm)
		if err != nil {
			t.Fatal(err)
		}
		return &DocumentSnapshot{
			Ref: ref,
			proto: &pb.Document{
				Name:       ref.Path,
				Fields:     map[string]*pb.Value{"foo": intval(value)},
				CreateTime: ts,
				UpdateTime: ts,
			},
			CreateTime: tm,
			UpdateTime: tm,
		}
	}
	change := func(ds *DocumentSnapshot) *pb.ListenResponse {
		return &pb.ListenResponse{ResponseType: &pb.ListenResponse_DocumentChange{&pb.DocumentChange{
			Document:  ds.proto,
			TargetIds: []int32{watchTargetID},
		}}}
	}

	del := func(ds *DocumentSnapshot) *pb.ListenResponse {
		return &pb.ListenResponse{ResponseType: &pb.ListenResponse_DocumentDelete{&pb.DocumentDelete{
			Document: ds.Ref.Path,
		}}}
	}

	q := Query{c: c, collectionID: "x"}
	current := &pb.ListenResponse{ResponseType: &pb.ListenResponse_TargetChange{&pb.TargetChange{
		TargetChangeType: pb.TargetChange_CURRENT,
	}}}
	noChange := &pb.ListenResponse{ResponseType: &pb.ListenResponse_TargetChange{&pb.TargetChange{
		TargetChangeType: pb.TargetChange_NO_CHANGE,
		ReadTime:         ptypes.TimestampNow(),
	}}}
	doc1 := doc("C/d1", 1, baseTime)
	doc1a := doc("C/d1", 2, baseTime.Add(time.Second))
	doc2 := doc("C/d2", 3, baseTime)
	for _, test := range []struct {
		desc      string
		responses []interface{}
		want      []*DocumentSnapshot
	}{
		{
			"no changes: empty btree",
			[]interface{}{current, noChange},
			nil,
		},
		{
			"add a doc",
			[]interface{}{change(doc1), current, noChange},
			[]*DocumentSnapshot{doc1},
		},
		{
			"add a doc, then remove it",
			[]interface{}{change(doc1), del(doc1), current, noChange},
			[]*DocumentSnapshot(nil),
		},
		{
			"add a doc, then add another one",
			[]interface{}{change(doc1), change(doc2), current, noChange},
			[]*DocumentSnapshot{doc1, doc2},
		},
		{
			"add a doc, then change it",
			[]interface{}{change(doc1), change(doc1a), current, noChange},
			[]*DocumentSnapshot{doc1a},
		},
	} {
		ws, err := newWatchStreamForQuery(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		request := &pb.ListenRequest{
			Database:     "projects/projectID/databases/(default)",
			TargetChange: &pb.ListenRequest_AddTarget{ws.target},
		}
		srv.addRPC(request, test.responses)
		tree, _, err := ws.nextSnapshot()
		if err != nil {
			t.Fatalf("%s: %v", test.desc, err)
		}
		got := treeDocs(tree)
		if diff := testDiff(got, test.want); diff != "" {
			t.Errorf("%s: %s", test.desc, diff)
		}
	}
}
