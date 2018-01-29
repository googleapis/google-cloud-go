// Copyright 2017 Google Inc. All Rights Reserved.
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
	"testing"

	"golang.org/x/net/context"

	"cloud.google.com/go/internal/pretty"
	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"

	"github.com/golang/protobuf/ptypes/wrappers"
)

func TestQueryToProto(t *testing.T) {
	filtr := func(path []string, op string, val interface{}) *pb.StructuredQuery_Filter {
		f, err := filter{path, op, val}.toProto()
		if err != nil {
			t.Fatal(err)
		}
		return f
	}

	c := &Client{projectID: "P", databaseID: "DB"}
	coll := c.Collection("C")
	q := coll.Query
	type S struct {
		A int `firestore:"a"`
	}
	docsnap := &DocumentSnapshot{
		Ref: coll.Doc("D"),
		proto: &pb.Document{
			Fields: map[string]*pb.Value{"a": intval(7), "b": intval(8)},
		},
	}
	for _, test := range []struct {
		desc string
		in   Query
		want *pb.StructuredQuery
	}{
		{
			desc: "q.Select()",
			in:   q.Select(),
			want: &pb.StructuredQuery{
				Select: &pb.StructuredQuery_Projection{
					Fields: []*pb.StructuredQuery_FieldReference{fref1("__name__")},
				},
			},
		},
		{
			desc: `q.Select("a", "b")`,
			in:   q.Select("a", "b"),
			want: &pb.StructuredQuery{
				Select: &pb.StructuredQuery_Projection{
					Fields: []*pb.StructuredQuery_FieldReference{fref1("a"), fref1("b")},
				},
			},
		},
		{
			desc: `q.Select("a", "b").Select("c")`,
			in:   q.Select("a", "b").Select("c"), // last wins
			want: &pb.StructuredQuery{
				Select: &pb.StructuredQuery_Projection{
					Fields: []*pb.StructuredQuery_FieldReference{fref1("c")},
				},
			},
		},
		{
			desc: `q.SelectPaths([]string{"*"}, []string{"/"})`,
			in:   q.SelectPaths([]string{"*"}, []string{"/"}),
			want: &pb.StructuredQuery{
				Select: &pb.StructuredQuery_Projection{
					Fields: []*pb.StructuredQuery_FieldReference{fref1("*"), fref1("/")},
				},
			},
		},
		{
			desc: `  q.Where("a", ">", 5)`,
			in:   q.Where("a", ">", 5),
			want: &pb.StructuredQuery{Where: filtr([]string{"a"}, ">", 5)},
		},
		{
			desc: `q.Where("a", ">", 5).Where("b", "<", "foo")`,
			in:   q.Where("a", ">", 5).Where("b", "<", "foo"),
			want: &pb.StructuredQuery{
				Where: &pb.StructuredQuery_Filter{
					&pb.StructuredQuery_Filter_CompositeFilter{
						&pb.StructuredQuery_CompositeFilter{
							Op: pb.StructuredQuery_CompositeFilter_AND,
							Filters: []*pb.StructuredQuery_Filter{
								filtr([]string{"a"}, ">", 5), filtr([]string{"b"}, "<", "foo"),
							},
						},
					},
				},
			},
		},
		{
			desc: `  q.WherePath([]string{"/", "*"}, ">", 5)`,
			in:   q.WherePath([]string{"/", "*"}, ">", 5),
			want: &pb.StructuredQuery{Where: filtr([]string{"/", "*"}, ">", 5)},
		},
		{
			desc: `q.OrderBy("b", Asc).OrderBy("a", Desc).OrderByPath([]string{"~"}, Asc)`,
			in:   q.OrderBy("b", Asc).OrderBy("a", Desc).OrderByPath([]string{"~"}, Asc),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("b"), pb.StructuredQuery_ASCENDING},
					{fref1("a"), pb.StructuredQuery_DESCENDING},
					{fref1("~"), pb.StructuredQuery_ASCENDING},
				},
			},
		},
		{
			desc: `q.Offset(2).Limit(3)`,
			in:   q.Offset(2).Limit(3),
			want: &pb.StructuredQuery{
				Offset: 2,
				Limit:  &wrappers.Int32Value{3},
			},
		},
		{
			desc: `q.Offset(2).Limit(3).Limit(4).Offset(5)`,
			in:   q.Offset(2).Limit(3).Limit(4).Offset(5), // last wins
			want: &pb.StructuredQuery{
				Offset: 5,
				Limit:  &wrappers.Int32Value{4},
			},
		},
		{
			desc: `q.OrderBy("a", Asc).StartAt(7).EndBefore(9)`,
			in:   q.OrderBy("a", Asc).StartAt(7).EndBefore(9),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7)},
					Before: true,
				},
				EndAt: &pb.Cursor{
					Values: []*pb.Value{intval(9)},
					Before: true,
				},
			},
		},
		{
			desc: `q.OrderBy("a", Asc).StartAt(7).EndAt(9)`,
			in:   q.OrderBy("a", Asc).StartAt(7).EndAt(9),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7)},
					Before: true,
				},
				EndAt: &pb.Cursor{
					Values: []*pb.Value{intval(9)},
					Before: false,
				},
			},
		},
		{
			desc: `q.OrderBy("a", Asc).StartAfter(7).EndAt(9)`,
			in:   q.OrderBy("a", Asc).StartAfter(7).EndAt(9),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7)},
					Before: false,
				},
				EndAt: &pb.Cursor{
					Values: []*pb.Value{intval(9)},
					Before: false,
				},
			},
		},
		{
			desc: `q.OrderBy(DocumentID, Asc).StartAfter("foo").EndBefore("bar")`,
			in:   q.OrderBy(DocumentID, Asc).StartAfter("foo").EndBefore("bar"),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{refval(coll.parentPath + "/documents/C/foo")},
					Before: false,
				},
				EndAt: &pb.Cursor{
					Values: []*pb.Value{refval(coll.parentPath + "/documents/C/bar")},
					Before: true,
				},
			},
		},
		{
			desc: `q.OrderBy("a", Asc).OrderBy("b", Desc).StartAfter(7, 8).EndAt(9, 10)`,
			in:   q.OrderBy("a", Asc).OrderBy("b", Desc).StartAfter(7, 8).EndAt(9, 10),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
					{fref1("b"), pb.StructuredQuery_DESCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7), intval(8)},
					Before: false,
				},
				EndAt: &pb.Cursor{
					Values: []*pb.Value{intval(9), intval(10)},
					Before: false,
				},
			},
		},
		{
			// last of StartAt/After wins, same for End
			desc: `q.OrderBy("a", Asc).StartAfter(1).StartAt(2).EndAt(3).EndBefore(4)`,
			in: q.OrderBy("a", Asc).
				StartAfter(1).StartAt(2).
				EndAt(3).EndBefore(4),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(2)},
					Before: true,
				},
				EndAt: &pb.Cursor{
					Values: []*pb.Value{intval(4)},
					Before: true,
				},
			},
		},
		// Start/End with DocumentSnapshot
		// These tests are from the "Document Snapshot Cursors" doc.
		{
			desc: `q.StartAt(docsnap)`,
			in:   q.StartAt(docsnap),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},
		{
			desc: `q.OrderBy("a", Asc).StartAt(docsnap)`,
			in:   q.OrderBy("a", Asc).StartAt(docsnap),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7), refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},

		{
			desc: `q.OrderBy("a", Desc).StartAt(docsnap)`,
			in:   q.OrderBy("a", Desc).StartAt(docsnap),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_DESCENDING},
					{fref1("__name__"), pb.StructuredQuery_DESCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7), refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},
		{
			desc: `q.OrderBy("a", Desc).OrderBy("b", Asc).StartAt(docsnap)`,
			in:   q.OrderBy("a", Desc).OrderBy("b", Asc).StartAt(docsnap),
			want: &pb.StructuredQuery{
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_DESCENDING},
					{fref1("b"), pb.StructuredQuery_ASCENDING},
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7), intval(8), refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},
		{
			desc: `q.Where("a", "==", 3).StartAt(docsnap)`,
			in:   q.Where("a", "==", 3).StartAt(docsnap),
			want: &pb.StructuredQuery{
				Where: filtr([]string{"a"}, "==", 3),
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},
		{
			desc: `q.Where("a", "<", 3).StartAt(docsnap)`,
			in:   q.Where("a", "<", 3).StartAt(docsnap),
			want: &pb.StructuredQuery{
				Where: filtr([]string{"a"}, "<", 3),
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7), refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},
		{
			desc: `q.Where("b", "==", 1).Where("a", "<", 3).StartAt(docsnap)`,
			in:   q.Where("b", "==", 1).Where("a", "<", 3).StartAt(docsnap),
			want: &pb.StructuredQuery{
				Where: &pb.StructuredQuery_Filter{
					&pb.StructuredQuery_Filter_CompositeFilter{
						&pb.StructuredQuery_CompositeFilter{
							Op: pb.StructuredQuery_CompositeFilter_AND,
							Filters: []*pb.StructuredQuery_Filter{
								filtr([]string{"b"}, "==", 1),
								filtr([]string{"a"}, "<", 3),
							},
						},
					},
				},
				OrderBy: []*pb.StructuredQuery_Order{
					{fref1("a"), pb.StructuredQuery_ASCENDING},
					{fref1("__name__"), pb.StructuredQuery_ASCENDING},
				},
				StartAt: &pb.Cursor{
					Values: []*pb.Value{intval(7), refval(coll.parentPath + "/documents/C/D")},
					Before: true,
				},
			},
		},
	} {
		got, err := test.in.toProto()
		if err != nil {
			t.Errorf("%s: %v", test.desc, err)
			continue
		}
		test.want.From = []*pb.StructuredQuery_CollectionSelector{{CollectionId: "C"}}
		if !testEqual(got, test.want) {
			t.Errorf("%s:\ngot\n%v\nwant\n%v", test.desc, pretty.Value(got), pretty.Value(test.want))
		}
	}
}

func fref1(s string) *pb.StructuredQuery_FieldReference {
	return fref([]string{s})
}

func TestQueryToProtoErrors(t *testing.T) {
	st := map[string]interface{}{"a": ServerTimestamp}
	del := map[string]interface{}{"a": Delete}
	c := &Client{projectID: "P", databaseID: "DB"}
	coll := c.Collection("C")
	docsnap := &DocumentSnapshot{
		Ref: coll.Doc("D"),
		proto: &pb.Document{
			Fields: map[string]*pb.Value{"a": intval(7)},
		},
	}
	q := coll.Query
	for _, query := range []Query{
		Query{},                                // no collection ID
		q.Where("x", "!=", 1),                  // invalid operator
		q.Where("~", ">", 1),                   // invalid path
		q.WherePath([]string{"*", ""}, ">", 1), // invalid path
		q.StartAt(1),                           // no OrderBy
		q.StartAt(2).OrderBy("x", Asc).OrderBy("y", Desc), // wrong # OrderBy
		q.Select("*"),                                     // invalid path
		q.SelectPaths([]string{"/", "", "~"}),             // invalid path
		q.OrderBy("[", Asc),                               // invalid path
		q.OrderByPath([]string{""}, Desc),                 // invalid path
		q.Where("x", "==", st),                            // ServerTimestamp in filter
		q.OrderBy("a", Asc).StartAt(st),                   // ServerTimestamp in Start
		q.OrderBy("a", Asc).EndAt(st),                     // ServerTimestamp in End
		q.Where("x", "==", del),                           // Delete in filter
		q.OrderBy("a", Asc).StartAt(del),                  // Delete in Start
		q.OrderBy("a", Asc).EndAt(del),                    // Delete in End
		q.OrderBy(DocumentID, Asc).StartAt(7),             // wrong type for __name__
		q.OrderBy(DocumentID, Asc).EndAt(7),               // wrong type for __name__
		q.OrderBy("b", Asc).StartAt(docsnap),              // doc snapshot does not have order-by field
		q.StartAt(docsnap).EndAt("x"),                     // mixed doc snapshot and fields
		q.StartAfter("x").EndBefore(docsnap),              // mixed doc snapshot and fields
	} {
		_, err := query.toProto()
		if err == nil {
			t.Errorf("%+v: got nil, want error", query)
		}
	}
}

func TestQueryMethodsDoNotModifyReceiver(t *testing.T) {
	var empty Query

	q := Query{}
	_ = q.Select("a", "b")
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}

	q = Query{}
	q1 := q.Where("a", ">", 3)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}
	// Extra check because Where appends to a slice.
	q1before := q.Where("a", ">", 3) // same as q1
	_ = q1.Where("b", "<", "foo")
	if !testEqual(q1, q1before) {
		t.Errorf("got %+v, want %+v", q1, q1before)
	}

	q = Query{}
	q1 = q.OrderBy("a", Asc)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}
	// Extra check because Where appends to a slice.
	q1before = q.OrderBy("a", Asc) // same as q1
	_ = q1.OrderBy("b", Desc)
	if !testEqual(q1, q1before) {
		t.Errorf("got %+v, want %+v", q1, q1before)
	}

	q = Query{}
	_ = q.Offset(5)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}

	q = Query{}
	_ = q.Limit(5)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}

	q = Query{}
	_ = q.StartAt(5)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}

	q = Query{}
	_ = q.StartAfter(5)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}

	q = Query{}
	_ = q.EndAt(5)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}

	q = Query{}
	_ = q.EndBefore(5)
	if !testEqual(q, empty) {
		t.Errorf("got %+v, want empty", q)
	}
}

func TestQueryFromCollectionRef(t *testing.T) {
	c := &Client{}
	coll := c.Collection("C")
	got := coll.Select("x").Offset(8)
	want := Query{
		c:            c,
		parentPath:   c.path(),
		collectionID: "C",
		selection:    []FieldPath{{"x"}},
		offset:       8,
	}
	if !testEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestQueryGetAll(t *testing.T) {
	// This implicitly tests DocumentIterator as well.
	const dbPath = "projects/projectID/databases/(default)"
	ctx := context.Background()
	c, srv := newMock(t)
	docNames := []string{"C/a", "C/b"}
	wantPBDocs := []*pb.Document{
		{
			Name:       dbPath + "/documents/" + docNames[0],
			CreateTime: aTimestamp,
			UpdateTime: aTimestamp,
			Fields:     map[string]*pb.Value{"f": intval(2)},
		},
		{
			Name:       dbPath + "/documents/" + docNames[1],
			CreateTime: aTimestamp2,
			UpdateTime: aTimestamp3,
			Fields:     map[string]*pb.Value{"f": intval(1)},
		},
	}

	srv.addRPC(nil, []interface{}{
		&pb.RunQueryResponse{Document: wantPBDocs[0]},
		&pb.RunQueryResponse{Document: wantPBDocs[1]},
	})
	gotDocs, err := c.Collection("C").Documents(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(gotDocs), len(wantPBDocs); got != want {
		t.Errorf("got %d docs, wanted %d", got, want)
	}
	for i, got := range gotDocs {
		want, err := newDocumentSnapshot(c.Doc(docNames[i]), wantPBDocs[i], c)
		if err != nil {
			t.Fatal(err)
		}
		if !testEqual(got, want) {
			// avoid writing a cycle
			got.c = nil
			want.c = nil
			t.Errorf("#%d: got %+v, want %+v", i, pretty.Value(got), pretty.Value(want))
		}
	}
}
