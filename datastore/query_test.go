// Copyright 2014 Google LLC
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

package datastore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var (
	key1 = &pb.Key{
		Path: []*pb.Key_PathElement{
			{
				Kind:   "Gopher",
				IdType: &pb.Key_PathElement_Id{Id: 6},
			},
		},
	}
	key2 = &pb.Key{
		Path: []*pb.Key_PathElement{
			{
				Kind:   "Gopher",
				IdType: &pb.Key_PathElement_Id{Id: 6},
			},
			{
				Kind:   "Gopher",
				IdType: &pb.Key_PathElement_Id{Id: 8},
			},
		},
	}
	countAlias = "count"
)

type fakeClient struct {
	pb.DatastoreClient
	queryFn    func(*pb.RunQueryRequest) (*pb.RunQueryResponse, error)
	commitFn   func(*pb.CommitRequest) (*pb.CommitResponse, error)
	aggQueryFn func(*pb.RunAggregationQueryRequest) (*pb.RunAggregationQueryResponse, error)
}

func (c *fakeClient) RunQuery(_ context.Context, req *pb.RunQueryRequest, _ ...grpc.CallOption) (*pb.RunQueryResponse, error) {
	return c.queryFn(req)
}

func (c *fakeClient) Commit(_ context.Context, req *pb.CommitRequest, _ ...grpc.CallOption) (*pb.CommitResponse, error) {
	return c.commitFn(req)
}

func (c *fakeClient) RunAggregationQuery(_ context.Context, req *pb.RunAggregationQueryRequest, _ ...grpc.CallOption) (*pb.RunAggregationQueryResponse, error) {
	return c.aggQueryFn(req)
}

func fakeRunQuery(in *pb.RunQueryRequest) (*pb.RunQueryResponse, error) {
	expectedIn := &pb.RunQueryRequest{
		QueryType: &pb.RunQueryRequest_Query{Query: &pb.Query{
			Kind: []*pb.KindExpression{{Name: "Gopher"}},
		}},
	}
	if !proto.Equal(in, expectedIn) {
		return nil, fmt.Errorf("unsupported argument: got %v want %v", in, expectedIn)
	}
	return &pb.RunQueryResponse{
		Batch: &pb.QueryResultBatch{
			MoreResults:      pb.QueryResultBatch_NO_MORE_RESULTS,
			EntityResultType: pb.EntityResult_FULL,
			EntityResults: []*pb.EntityResult{
				{
					Entity: &pb.Entity{
						Key: key1,
						Properties: map[string]*pb.Value{
							"Name":   {ValueType: &pb.Value_StringValue{StringValue: "George"}},
							"Height": {ValueType: &pb.Value_IntegerValue{IntegerValue: 32}},
						},
					},
				},
				{
					Entity: &pb.Entity{
						Key: key2,
						Properties: map[string]*pb.Value{
							"Name": {ValueType: &pb.Value_StringValue{StringValue: "Rufus"}},
							// No height for Rufus.
						},
					},
				},
			},
		},
	}, nil
}

func fakeRunAggregationQuery(req *pb.RunAggregationQueryRequest) (*pb.RunAggregationQueryResponse, error) {
	expectedIn := &pb.RunAggregationQueryRequest{
		QueryType: &pb.RunAggregationQueryRequest_AggregationQuery{
			AggregationQuery: &pb.AggregationQuery{
				QueryType: &pb.AggregationQuery_NestedQuery{
					NestedQuery: &pb.Query{
						Kind: []*pb.KindExpression{{Name: "Gopher"}},
					},
				},
				Aggregations: []*pb.AggregationQuery_Aggregation{
					{
						Operator: &pb.AggregationQuery_Aggregation_Count_{},
						Alias:    countAlias,
					},
				},
			},
		},
	}
	if !proto.Equal(req, expectedIn) {
		return nil, fmt.Errorf("unsupported argument: got %v want %v", req, expectedIn)
	}
	return &pb.RunAggregationQueryResponse{
		Batch: &pb.AggregationResultBatch{
			AggregationResults: []*pb.AggregationResult{
				{
					AggregateProperties: map[string]*pb.Value{
						"count": {
							ValueType: &pb.Value_IntegerValue{IntegerValue: 1},
						},
					},
				},
			},
		},
	}, nil
}

type StructThatImplementsPLS struct{}

func (StructThatImplementsPLS) Load(p []Property) error   { return nil }
func (StructThatImplementsPLS) Save() ([]Property, error) { return nil, nil }

var _ PropertyLoadSaver = StructThatImplementsPLS{}

type StructPtrThatImplementsPLS struct{}

func (*StructPtrThatImplementsPLS) Load(p []Property) error   { return nil }
func (*StructPtrThatImplementsPLS) Save() ([]Property, error) { return nil, nil }

var _ PropertyLoadSaver = &StructPtrThatImplementsPLS{}

type PropertyMap map[string]Property

func (m PropertyMap) Load(props []Property) error {
	for _, p := range props {
		m[p.Name] = p
	}
	return nil
}

func (m PropertyMap) Save() ([]Property, error) {
	props := make([]Property, 0, len(m))
	for _, p := range m {
		props = append(props, p)
	}
	return props, nil
}

var _ PropertyLoadSaver = PropertyMap{}

type Gopher struct {
	Name   string
	Height int
}

// typeOfEmptyInterface is the type of interface{}, but we can't use
// reflect.TypeOf((interface{})(nil)) directly because TypeOf takes an
// interface{}.
var typeOfEmptyInterface = reflect.TypeOf((*interface{})(nil)).Elem()

func TestCheckMultiArg(t *testing.T) {
	testCases := []struct {
		v        interface{}
		mat      multiArgType
		elemType reflect.Type
	}{
		// Invalid cases.
		{nil, multiArgTypeInvalid, nil},
		{Gopher{}, multiArgTypeInvalid, nil},
		{&Gopher{}, multiArgTypeInvalid, nil},
		{PropertyList{}, multiArgTypeInvalid, nil}, // This is a special case.
		{PropertyMap{}, multiArgTypeInvalid, nil},
		{[]*PropertyList(nil), multiArgTypeInvalid, nil},
		{[]*PropertyMap(nil), multiArgTypeInvalid, nil},
		{[]**Gopher(nil), multiArgTypeInvalid, nil},
		{[]*interface{}(nil), multiArgTypeInvalid, nil},
		// Valid cases.
		{
			[]PropertyList(nil),
			multiArgTypePropertyLoadSaver,
			reflect.TypeOf(PropertyList{}),
		},
		{
			[]PropertyMap(nil),
			multiArgTypePropertyLoadSaver,
			reflect.TypeOf(PropertyMap{}),
		},
		{
			[]StructThatImplementsPLS(nil),
			multiArgTypePropertyLoadSaver,
			reflect.TypeOf(StructThatImplementsPLS{}),
		},
		{
			[]StructPtrThatImplementsPLS(nil),
			multiArgTypePropertyLoadSaver,
			reflect.TypeOf(StructPtrThatImplementsPLS{}),
		},
		{
			[]Gopher(nil),
			multiArgTypeStruct,
			reflect.TypeOf(Gopher{}),
		},
		{
			[]*Gopher(nil),
			multiArgTypeStructPtr,
			reflect.TypeOf(Gopher{}),
		},
		{
			[]interface{}(nil),
			multiArgTypeInterface,
			typeOfEmptyInterface,
		},
	}
	for _, tc := range testCases {
		mat, elemType := checkMultiArg(reflect.ValueOf(tc.v))
		if mat != tc.mat || elemType != tc.elemType {
			t.Errorf("checkMultiArg(%T): got %v, %v want %v, %v",
				tc.v, mat, elemType, tc.mat, tc.elemType)
		}
	}
}

func TestSimpleQuery(t *testing.T) {
	struct1 := Gopher{Name: "George", Height: 32}
	struct2 := Gopher{Name: "Rufus"}
	pList1 := PropertyList{
		{
			Name:  "Height",
			Value: int64(32),
		},
		{
			Name:  "Name",
			Value: "George",
		},
	}
	pList2 := PropertyList{
		{
			Name:  "Name",
			Value: "Rufus",
		},
	}
	pMap1 := PropertyMap{
		"Name": Property{
			Name:  "Name",
			Value: "George",
		},
		"Height": Property{
			Name:  "Height",
			Value: int64(32),
		},
	}
	pMap2 := PropertyMap{
		"Name": Property{
			Name:  "Name",
			Value: "Rufus",
		},
	}

	testCases := []struct {
		dst  interface{}
		want interface{}
	}{
		// The destination must have type *[]P, *[]S or *[]*S, for some non-interface
		// type P such that *P implements PropertyLoadSaver, or for some struct type S.
		{new([]Gopher), &[]Gopher{struct1, struct2}},
		{new([]*Gopher), &[]*Gopher{&struct1, &struct2}},
		{new([]PropertyList), &[]PropertyList{pList1, pList2}},
		{new([]PropertyMap), &[]PropertyMap{pMap1, pMap2}},

		// Any other destination type is invalid.
		{0, nil},
		{Gopher{}, nil},
		{PropertyList{}, nil},
		{PropertyMap{}, nil},
		{[]int{}, nil},
		{[]Gopher{}, nil},
		{[]PropertyList{}, nil},
		{new(int), nil},
		{new(Gopher), nil},
		{new(PropertyList), nil}, // This is a special case.
		{new(PropertyMap), nil},
		{new([]int), nil},
		{new([]map[int]int), nil},
		{new([]map[string]Property), nil},
		{new([]map[string]interface{}), nil},
		{new([]*int), nil},
		{new([]*map[int]int), nil},
		{new([]*map[string]Property), nil},
		{new([]*map[string]interface{}), nil},
		{new([]**Gopher), nil},
		{new([]*PropertyList), nil},
		{new([]*PropertyMap), nil},
	}
	for _, tc := range testCases {
		nCall := 0
		client := &Client{
			client: &fakeClient{
				queryFn: func(req *pb.RunQueryRequest) (*pb.RunQueryResponse, error) {
					nCall++
					return fakeRunQuery(req)
				},
			},
		}
		ctx := context.Background()

		var (
			expectedErr   error
			expectedNCall int
		)
		if tc.want == nil {
			expectedErr = ErrInvalidEntityType
		} else {
			expectedNCall = 1
		}
		keys, err := client.GetAll(ctx, NewQuery("Gopher"), tc.dst)
		if err != expectedErr {
			t.Errorf("dst type %T: got error %v, want %v", tc.dst, err, expectedErr)
			continue
		}
		if nCall != expectedNCall {
			t.Errorf("dst type %T: Context.Call was called an incorrect number of times: got %d want %d", tc.dst, nCall, expectedNCall)
			continue
		}
		if err != nil {
			continue
		}

		key1 := IDKey("Gopher", 6, nil)
		expectedKeys := []*Key{
			key1,
			IDKey("Gopher", 8, key1),
		}
		if l1, l2 := len(keys), len(expectedKeys); l1 != l2 {
			t.Errorf("dst type %T: got %d keys, want %d keys", tc.dst, l1, l2)
			continue
		}
		for i, key := range keys {
			if !keysEqual(key, expectedKeys[i]) {
				t.Errorf("dst type %T: got key #%d %v, want %v", tc.dst, i, key, expectedKeys[i])
				continue
			}
		}

		// Make sure we sort any PropertyList items (the order is not deterministic).
		if pLists, ok := tc.dst.(*[]PropertyList); ok {
			for _, p := range *pLists {
				sort.Sort(byName(p))
			}
		}

		if !testutil.Equal(tc.dst, tc.want) {
			t.Errorf("dst type %T: Entities\ngot  %+v\nwant %+v", tc.dst, tc.dst, tc.want)
			continue
		}
	}
}

// keysEqual is like (*Key).Equal, but ignores the App ID.
func keysEqual(a, b *Key) bool {
	for a != nil && b != nil {
		if a.Kind != b.Kind || a.Name != b.Name || a.ID != b.ID {
			return false
		}
		a, b = a.Parent, b.Parent
	}
	return a == b
}

func TestQueriesAreImmutable(t *testing.T) {
	// Test that deriving q2 from q1 does not modify q1.
	q0 := NewQuery("foo")
	q1 := NewQuery("foo")
	q2 := q1.Offset(2)
	if !testutil.Equal(q0, q1, cmp.AllowUnexported(Query{})) {
		t.Errorf("q0 and q1 were not equal")
	}
	if testutil.Equal(q1, q2, cmp.AllowUnexported(Query{})) {
		t.Errorf("q1 and q2 were equal")
	}

	// Test that deriving from q4 twice does not conflict, even though
	// q4 has a long list of order clauses. This tests that the arrays
	// backed by a query's slice of orders are not shared.
	f := func() *Query {
		q := NewQuery("bar")
		// 47 is an ugly number that is unlikely to be near a re-allocation
		// point in repeated append calls. For example, it's not near a power
		// of 2 or a multiple of 10.
		for i := 0; i < 47; i++ {
			q = q.Order(fmt.Sprintf("x%d", i))
		}
		return q
	}
	q3 := f().Order("y")
	q4 := f()
	q5 := q4.Order("y")
	q6 := q4.Order("z")
	if !testutil.Equal(q3, q5, cmp.AllowUnexported(Query{})) {
		t.Errorf("q3 and q5 were not equal")
	}
	if testutil.Equal(q5, q6, cmp.AllowUnexported(Query{})) {
		t.Errorf("q5 and q6 were equal")
	}
}

type testFilterCase struct {
	filterStr     string
	fieldName     string
	operator      string
	wantOp        operator
	wantFieldName string
}

var (
	/*
		// Quoted and interesting field names.


	*/

	// Supported ops both filters.
	filterTestCases = []testFilterCase{
		{"x<", "x", "<", lessThan, "x"},
		{"x <", "x", "<", lessThan, "x"},
		{"x  <", "x", "<", lessThan, "x"},
		{"   x   <  ", "x", "<", lessThan, "x"},
		{"x <=", "x", "<=", lessEq, "x"},
		{"x =", "x", "=", equal, "x"},
		{"x >=", "x", ">=", greaterEq, "x"},
		{"x >", "x", ">", greaterThan, "x"},
		{"in >", "in", ">", greaterThan, "in"},
		{"in>", "in", ">", greaterThan, "in"},
		{"x!=", "x", "!=", notEqual, "x"},
		{"x !=", "x", "!=", notEqual, "x"},
		{" x  !=  ", "x", "!=", notEqual, "x"},
		{"x > y =", "x > y", "=", equal, "x > y"},
		{"` x ` =", " x ", "=", equal, " x "},
		{`" x " =`, " x ", "=", equal, " x "},
		{`" \"x " =`, ` "x `, "=", equal, ` "x `},
	}
	// Supported in FilterField only.
	filterFieldTestCases = []testFilterCase{
		{"x in", "x", "in", in, "x"},
		{"x not-in", "x", "not-in", notIn, "x"},
		{"ins in", "ins", "in", in, "ins"},
		{"in not-in", "in", "not-in", notIn, "in"},
	}
	// Operators not supported in either filter method
	filterUnsupported = []testFilterCase{
		{filterStr: "x IN", fieldName: "x", operator: "IN", wantOp: "", wantFieldName: ""},
		{filterStr: "x NOT-IN", fieldName: "x", operator: "NOT-IN", wantOp: "", wantFieldName: ""},
		{filterStr: "x EQ", fieldName: "x", operator: "EQ", wantOp: "", wantFieldName: ""},
		{filterStr: "x lt", fieldName: "x", operator: "lt", wantOp: "", wantFieldName: ""},
		{filterStr: "x <>", fieldName: "x", operator: "<>", wantOp: "", wantFieldName: ""},
		{filterStr: "x >>", fieldName: "x", operator: ">>", wantOp: "", wantFieldName: ""},
		{filterStr: "x ==", fieldName: "x", operator: "==", wantOp: "", wantFieldName: ""},
		{filterStr: "x =<", fieldName: "x", operator: "=<", wantOp: "", wantFieldName: ""},
		{filterStr: "x =>", fieldName: "x", operator: "=>", wantOp: "", wantFieldName: ""},
		{filterStr: "x !", fieldName: "x", operator: "!", wantOp: "", wantFieldName: ""},
		{filterStr: "x ", fieldName: "x", operator: "", wantOp: "", wantFieldName: ""},
		{filterStr: "x", fieldName: "x", operator: "", wantOp: "", wantFieldName: ""},
		{filterStr: `" x =`, fieldName: `" x`, operator: "=", wantOp: "", wantFieldName: ""},
		{filterStr: `" x ="`, fieldName: `" x `, operator: `="`, wantOp: "", wantFieldName: ""},
		{filterStr: "` x \" =", fieldName: "` x \"", operator: "=", wantOp: "", wantFieldName: ""},
	}
)

type pfToProtoTestCase struct {
	pf         PropertyFilter
	wantErrMsg string
}

func TestPropertyFilterToProto(t *testing.T) {

	testCases := []pfToProtoTestCase{
		{PropertyFilter{FieldName: "x", Operator: "=", Value: 4}, ""},
		{PropertyFilter{FieldName: "x ", Operator: "=", Value: 4}, ""},
		{PropertyFilter{FieldName: "", Operator: "=", Value: 4}, "datastore: empty query filter field name"},
		{PropertyFilter{FieldName: "x", Operator: "==", Value: 4}, "datastore: invalid operator \"==\" in filter"},
		{PropertyFilter{FieldName: "x", Operator: "==", Value: struct{ x string }{x: "sample"}}, "datastore: bad query filter value type: invalid Value type struct { x string }"},
	}

	successFilterFieldTestCases := append(filterTestCases, filterFieldTestCases...)
	for _, sfftc := range successFilterFieldTestCases {
		testCases = append(testCases, pfToProtoTestCase{
			PropertyFilter{FieldName: sfftc.fieldName, Operator: sfftc.operator, Value: 4}, "",
		})
	}

	for _, tc := range testCases {
		_, err := tc.pf.toProto()
		gotErrMsg := ""
		if err != nil {
			gotErrMsg = err.Error()
		}

		if gotErrMsg != tc.wantErrMsg {
			t.Errorf("PropertyFilter proto conversion error: \nwant %v,\ngot %v", tc.wantErrMsg, gotErrMsg)
		}
	}
}

func TestCompositeFilterToProto(t *testing.T) {
	testCases := []struct {
		cf         CompositeFilter
		wantErrMsg string
	}{
		{AndFilter{
			[]EntityFilter{
				PropertyFilter{FieldName: "x", Operator: "=", Value: 4},
				PropertyFilter{FieldName: "y", Operator: "<", Value: 3},
			},
		}, ""},
		{OrFilter{
			[]EntityFilter{
				PropertyFilter{FieldName: "x", Operator: "=", Value: 4},
				PropertyFilter{FieldName: "y", Operator: "<", Value: 3},
			},
		}, ""},

		// Fail when inner filter is malformed
		{AndFilter{
			[]EntityFilter{
				PropertyFilter{FieldName: "x", Operator: "==", Value: 4},
				PropertyFilter{FieldName: "y", Operator: "<", Value: 3},
			},
		}, "datastore: invalid operator \"==\" in filter"},
		{OrFilter{
			[]EntityFilter{
				PropertyFilter{FieldName: "x", Operator: "==", Value: 4},
				PropertyFilter{FieldName: "y", Operator: "<", Value: 3},
			},
		}, "datastore: invalid operator \"==\" in filter"},
	}
	for _, tc := range testCases {
		_, gotErr := tc.cf.toProto()
		gotErrMsg := ""
		if gotErr != nil {
			gotErrMsg = gotErr.Error()
		}

		if gotErrMsg != tc.wantErrMsg {
			t.Errorf("CompositeFilter proto conversion error: \nwant %v,\ngot %v", tc.wantErrMsg, gotErrMsg)
		}

	}
}

func TestFilterEntity(t *testing.T) {
	testCases := []struct {
		ef         EntityFilter
		wantErrMsg string
	}{
		{AndFilter{
			[]EntityFilter{
				PropertyFilter{FieldName: "x", Operator: "==", Value: 4},
				PropertyFilter{FieldName: "y", Operator: "<", Value: 3},
			},
		}, "datastore: invalid operator \"==\" in filter"},
		{AndFilter{
			[]EntityFilter{
				PropertyFilter{FieldName: "`x", Operator: "=", Value: 4},
				PropertyFilter{FieldName: "y", Operator: "<", Value: 3},
			},
		}, "datastore: invalid syntax for quoted field name \"`x\""},
	}

	for _, tc := range testCases {
		q := NewQuery("foo").FilterEntity(tc.ef)
		gotErrMsg := ""
		if q.err != nil {
			gotErrMsg = q.err.Error()
		}
		if gotErrMsg != tc.wantErrMsg {
			t.Errorf("FilterEntity error: \nwant %v,\ngot %v", tc.wantErrMsg, gotErrMsg)
		}
	}
}

func TestFilterParser(t *testing.T) {
	// Success cases
	for _, tc := range filterTestCases {
		q := NewQuery("foo").Filter(tc.filterStr, 42)
		if q.err != nil {
			t.Errorf("%q: error=%v", tc.filterStr, q.err)
			continue
		}
		if len(q.filter) != 1 {
			t.Errorf("%q: len=%d, want %d", tc.filterStr, len(q.filter), 1)
			continue
		}
		got, want := q.filter[0], PropertyFilter{tc.wantFieldName, string(tc.wantOp), 42}
		if got != want {
			t.Errorf("%q: got %v, want %v", tc.filterStr, got, want)
			continue
		}
	}
	// Failure cases
	failureTestCases := append(filterFieldTestCases, filterUnsupported...)
	for _, tc := range failureTestCases {
		q := NewQuery("foo").Filter(tc.filterStr, 42)
		if q.err == nil {
			t.Errorf("%q: should have thrown error", tc.filterStr)
		}
	}
}

func TestFilterField(t *testing.T) {
	successTestCases := append(filterTestCases, filterFieldTestCases...)
	for _, tc := range successTestCases {
		q := NewQuery("foo").FilterField(tc.fieldName, tc.operator, 42)
		if q.err != nil {
			t.Errorf("%q %q: error: %v", tc.fieldName, tc.operator, q.err)
			continue
		}
		if len(q.filter) != 1 {
			t.Errorf("%q: len=%d, want %d", tc.fieldName, len(q.filter), 1)
			continue
		}
		got, want := q.filter[0], PropertyFilter{tc.fieldName, string(tc.wantOp), 42}
		if got != want {
			t.Errorf("%q %q: got %v, want %v", tc.fieldName, tc.operator, got, want)
			continue
		}
	}
	for _, tc := range filterUnsupported {
		q := NewQuery("foo").Filter(tc.filterStr, 42)
		if q.err == nil {
			t.Errorf("%q: should have thrown error", tc.filterStr)
		}
	}
}

func TestUnquote(t *testing.T) {
	testCases := []struct {
		input string
		want  string
	}{
		{`" x "`, ` x `},
		{`"\" \\\"x \""`, `" \"x "`},
	}

	for _, tc := range testCases {
		got, err := unquote(tc.input)

		if err != nil {
			t.Errorf("error parsing field name: %v", err)
		}

		if got != tc.want {
			t.Errorf("field name parsing error: \nwant %v,\ngot %v", tc.want, got)
		}
	}
}

func TestNamespaceQuery(t *testing.T) {
	gotNamespace := make(chan string, 1)
	ctx := context.Background()
	client := &Client{
		client: &fakeClient{
			queryFn: func(req *pb.RunQueryRequest) (*pb.RunQueryResponse, error) {
				if part := req.PartitionId; part != nil {
					gotNamespace <- part.NamespaceId
				} else {
					gotNamespace <- ""
				}
				return nil, errors.New("not implemented")
			},
		},
	}

	var gs []Gopher

	// Ignore errors for the rest of this test.
	client.GetAll(ctx, NewQuery("gopher"), &gs)
	if got, want := <-gotNamespace, ""; got != want {
		t.Errorf("GetAll: got namespace %q, want %q", got, want)
	}
	client.Count(ctx, NewQuery("gopher"))
	if got, want := <-gotNamespace, ""; got != want {
		t.Errorf("Count: got namespace %q, want %q", got, want)
	}

	const ns = "not_default"
	client.GetAll(ctx, NewQuery("gopher").Namespace(ns), &gs)
	if got, want := <-gotNamespace, ns; got != want {
		t.Errorf("GetAll: got namespace %q, want %q", got, want)
	}
	client.Count(ctx, NewQuery("gopher").Namespace(ns))
	if got, want := <-gotNamespace, ns; got != want {
		t.Errorf("Count: got namespace %q, want %q", got, want)
	}
}

func TestReadOptions(t *testing.T) {
	tid := []byte{1}
	for _, test := range []struct {
		q    *Query
		want *pb.ReadOptions
	}{
		{
			q:    NewQuery(""),
			want: nil,
		},
		{
			q:    NewQuery("").Transaction(nil),
			want: nil,
		},
		{
			q: NewQuery("").Transaction(&Transaction{id: tid}),
			want: &pb.ReadOptions{
				ConsistencyType: &pb.ReadOptions_Transaction{
					Transaction: tid,
				},
			},
		},
		{
			q: NewQuery("").EventualConsistency(),
			want: &pb.ReadOptions{
				ConsistencyType: &pb.ReadOptions_ReadConsistency_{
					ReadConsistency: pb.ReadOptions_EVENTUAL,
				},
			},
		},
	} {
		req := &pb.RunQueryRequest{}
		if err := test.q.toRunQueryRequest(req); err != nil {
			t.Fatalf("%+v: got %v, want no error", test.q, err)
		}
	}
	// Test errors.
	for _, q := range []*Query{
		NewQuery("").Transaction(&Transaction{id: nil, state: transactionStateExpired}),
		NewQuery("").Transaction(&Transaction{id: tid, state: transactionStateInProgress}).EventualConsistency(),
	} {
		req := &pb.RunQueryRequest{}
		if err := q.toRunQueryRequest(req); err == nil {
			t.Errorf("%+v: got nil, wanted error", q)
		}
	}
}

func TestInvalidFilters(t *testing.T) {
	client := &Client{
		client: &fakeClient{
			queryFn: func(req *pb.RunQueryRequest) (*pb.RunQueryResponse, error) {
				return fakeRunQuery(req)
			},
		},
	}

	// Used for an invalid type
	type MyType int
	var v MyType = 1

	for _, q := range []*Query{
		NewQuery("SomeKey").Filter("", 0),
		NewQuery("SomeKey").Filter("fld=", v),
	} {
		if _, err := client.Count(context.Background(), q); err == nil {
			t.Errorf("%+v: got nil, wanted error", q)
		}
	}
}

func TestRunErr(t *testing.T) {
	client := &Client{
		client: &fakeClient{
			queryFn: func(req *pb.RunQueryRequest) (*pb.RunQueryResponse, error) {
				fmt.Printf("received next request")
				return fakeRunQuery(req)
			},
		},
	}

	type Gopher struct {
		A int
	}

	ctx := context.Background()
	q := NewQuery("Gopher").Filter("", 2)
	it := client.Run(ctx, q)

	var g1 Gopher
	it.Next(&g1)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Cursor panic")
		}
	}()
	it.Cursor()
}

func TestAggregationQuery(t *testing.T) {
	client := &Client{
		client: &fakeClient{
			aggQueryFn: func(req *pb.RunAggregationQueryRequest) (*pb.RunAggregationQueryResponse, error) {
				return fakeRunAggregationQuery(req)
			},
		},
	}

	q := NewQuery("Gopher")
	aq := q.NewAggregationQuery()
	aq.WithCount(countAlias)

	res, err := client.RunAggregationQuery(context.Background(), aq)
	if err != nil {
		t.Fatal(err)
	}

	count, ok := res[countAlias]
	if !ok {
		t.Errorf("%s key does not exist in return aggregation result", countAlias)
	}

	want := &pb.Value{
		ValueType: &pb.Value_IntegerValue{IntegerValue: 1},
	}

	cv := count.(*pb.Value)
	if !proto.Equal(want, cv) {
		t.Errorf("want: %v\ngot: %v\n", want, cv)
	}
}

func TestAggregationQueryIsNil(t *testing.T) {
	client := &Client{
		client: &fakeClient{
			aggQueryFn: func(req *pb.RunAggregationQueryRequest) (*pb.RunAggregationQueryResponse, error) {
				return fakeRunAggregationQuery(req)
			},
		},
	}

	var q Query
	aq := q.NewAggregationQuery()
	_, err := client.RunAggregationQuery(context.Background(), aq)
	if err == nil {
		t.Fatal(err)
	}

	q2 := NewQuery("Gopher")
	aq2 := q2.NewAggregationQuery()
	_, err = client.RunAggregationQuery(context.Background(), aq2)
	if err == nil {
		t.Fatal(err)
	}

	aq3 := q2.NewAggregationQuery().WithCount("")
	_, err = client.RunAggregationQuery(context.Background(), aq3)
	if err == nil {
		t.Fatal(err)
	}
}

func TestExplainOptionsApply(t *testing.T) {
	pbExplainOptions := pb.ExplainOptions{
		Analyze: true,
	}
	for _, testcase := range []struct {
		desc            string
		existingOptions *pb.ExplainOptions
		newOptions      ExplainOptions
		wantErrMsg      string
	}{
		{
			desc:            "ExplainOptions specified multiple times",
			existingOptions: &pbExplainOptions,
			newOptions: ExplainOptions{
				Analyze: true,
			},
			wantErrMsg: "ExplainOptions can be specified only once",
		},
		{
			desc:            "ExplainOptions specified once",
			existingOptions: nil,
			newOptions: ExplainOptions{
				Analyze: true,
			},
		},
	} {
		gotErr := testcase.newOptions.apply(&runQuerySettings{explainOptions: testcase.existingOptions})
		if (gotErr == nil && testcase.wantErrMsg != "") ||
			(gotErr != nil && !strings.Contains(gotErr.Error(), testcase.wantErrMsg)) {
			t.Errorf("%v: apply got: %v want: %v", testcase.desc, gotErr.Error(), testcase.wantErrMsg)
		}
	}
}

func TestNewRunQuerySettings(t *testing.T) {
	for _, testcase := range []struct {
		desc       string
		opts       []RunOption
		wantErrMsg string
	}{
		{
			desc:       "nil RunOption",
			opts:       []RunOption{ExplainOptions{Analyze: true}, nil},
			wantErrMsg: "cannot be nil",
		},
		{
			desc: "success RunOption",
			opts: []RunOption{ExplainOptions{Analyze: true}},
		},
		{
			desc:       "ExplainOptions specified multiple times",
			opts:       []RunOption{ExplainOptions{Analyze: true}, ExplainOptions{Analyze: false}, ExplainOptions{Analyze: true}},
			wantErrMsg: "ExplainOptions can be specified only once",
		},
	} {
		_, gotErr := newRunQuerySettings(testcase.opts)
		if (gotErr == nil && testcase.wantErrMsg != "") ||
			(gotErr != nil && !strings.Contains(gotErr.Error(), testcase.wantErrMsg)) {
			t.Errorf("%v: newRunQuerySettings got: %v want: %v", testcase.desc, gotErr, testcase.wantErrMsg)
		}
	}
}

func TestValidateReadOptions(t *testing.T) {
	eventualInTxnErr := errEventualConsistencyTransaction

	for _, test := range []struct {
		desc     string
		eventual bool
		trans    *Transaction
		wantErr  error
	}{
		{
			desc:     "EventualConsistency query in a transaction",
			eventual: true,
			trans: &Transaction{
				id:    []byte("test id"),
				state: transactionStateInProgress,
			},
			wantErr: eventualInTxnErr,
		},
		{
			desc: "Expired transaction in non-eventual query",
			trans: &Transaction{
				id:    nil,
				state: transactionStateExpired,
			},
			wantErr: errExpiredTransaction,
		},
		{
			desc: "Expired transaction in eventual query",
			trans: &Transaction{
				id:    nil,
				state: transactionStateExpired,
			},
			eventual: true,
			wantErr:  eventualInTxnErr,
		},
		{
			desc: "No transaction in non-eventual query",
		},
		{
			desc:     "No transaction in eventual query",
			eventual: true,
		},
	} {
		gotErr := validateReadOptions(test.eventual, test.trans)
		gotErrMsg := ""
		if gotErr != nil {
			gotErrMsg = gotErr.Error()
		}

		wantErrMsg := ""
		if test.wantErr != nil {
			wantErrMsg = test.wantErr.Error()
		}

		if gotErrMsg != wantErrMsg {
			t.Errorf("%q: got: %v, want: %v", test.desc, gotErr, test.wantErr)
		}
	}
}
