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
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestQueryConstruction(t *testing.T) {
	tests := []struct {
		q, exp *Query
		err    string
	}{
		{
			q: NewQuery("Foo"),
			exp: &Query{
				kind:  "Foo",
				limit: -1,
			},
		},
		{
			// Regular filtered query with standard spacing.
			q: NewQuery("Foo").Filter("foo >", 7),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        greaterThan,
						Value:     7,
					},
				},
				limit: -1,
			},
		},
		{
			// Filtered query with no spacing.
			q: NewQuery("Foo").Filter("foo=", 6),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        equal,
						Value:     6,
					},
				},
				limit: -1,
			},
		},
		{
			// Filtered query with funky spacing.
			q: NewQuery("Foo").Filter(" foo< ", 8),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        lessThan,
						Value:     8,
					},
				},
				limit: -1,
			},
		},
		{
			// Filtered query with multicharacter op.
			q: NewQuery("Foo").Filter("foo >=", 9),
			exp: &Query{
				kind: "Foo",
				filter: []filter{
					{
						FieldName: "foo",
						Op:        greaterEq,
						Value:     9,
					},
				},
				limit: -1,
			},
		},
		{
			// Query with ordering.
			q: NewQuery("Foo").Order("bar"),
			exp: &Query{
				kind: "Foo",
				order: []order{
					{
						FieldName: "bar",
						Direction: ascending,
					},
				},
				limit: -1,
			},
		},
		{
			// Query with reverse ordering, and funky spacing.
			q: NewQuery("Foo").Order(" - bar"),
			exp: &Query{
				kind: "Foo",
				order: []order{
					{
						FieldName: "bar",
						Direction: descending,
					},
				},
				limit: -1,
			},
		},
		{
			// Query with an empty ordering.
			q:   NewQuery("Foo").Order(""),
			err: "empty order",
		},
		{
			// Query with a + ordering.
			q:   NewQuery("Foo").Order("+bar"),
			err: "invalid order",
		},
	}
	for i, test := range tests {
		if test.q.err != nil {
			got := test.q.err.Error()
			if !strings.Contains(got, test.err) {
				t.Errorf("%d: error mismatch: got %q want something containing %q", i, got, test.err)
			}
			continue
		}
		if !testutil.Equal(test.q, test.exp, cmp.AllowUnexported(Query{})) {
			t.Errorf("%d: mismatch: got %v want %v", i, test.q, test.exp)
		}
	}
}

func TestPutMultiTypes(t *testing.T) {
	ctx := context.Background()
	type S struct {
		A int
		B string
	}

	testCases := []struct {
		desc    string
		src     interface{}
		wantErr bool
	}{
		// Test cases to check each of the valid input types for src.
		// Each case has the same elements.
		{
			desc: "type []struct",
			src: []S{
				{1, "one"}, {2, "two"},
			},
		},
		{
			desc: "type []*struct",
			src: []*S{
				{1, "one"}, {2, "two"},
			},
		},
		{
			desc: "type []interface{} with PLS elems",
			src: []interface{}{
				&PropertyList{Property{Name: "A", Value: 1}, Property{Name: "B", Value: "one"}},
				&PropertyList{Property{Name: "A", Value: 2}, Property{Name: "B", Value: "two"}},
			},
		},
		{
			desc: "type []interface{} with struct ptr elems",
			src: []interface{}{
				&S{1, "one"}, &S{2, "two"},
			},
		},
		{
			desc: "type []PropertyLoadSaver{}",
			src: []PropertyLoadSaver{
				&PropertyList{Property{Name: "A", Value: 1}, Property{Name: "B", Value: "one"}},
				&PropertyList{Property{Name: "A", Value: 2}, Property{Name: "B", Value: "two"}},
			},
		},
		{
			desc: "type []P (non-pointer, *P implements PropertyLoadSaver)",
			src: []PropertyList{
				{Property{Name: "A", Value: 1}, Property{Name: "B", Value: "one"}},
				{Property{Name: "A", Value: 2}, Property{Name: "B", Value: "two"}},
			},
		},
		// Test some invalid cases.
		{
			desc: "type []interface{} with struct elems",
			src: []interface{}{
				S{1, "one"}, S{2, "two"},
			},
			wantErr: true,
		},
		{
			desc: "PropertyList",
			src: PropertyList{
				Property{Name: "A", Value: 1},
				Property{Name: "B", Value: "one"},
			},
			wantErr: true,
		},
		{
			desc:    "type []int",
			src:     []int{1, 2},
			wantErr: true,
		},
		{
			desc:    "not a slice",
			src:     S{1, "one"},
			wantErr: true,
		},
		{
			desc: "slice and key length is different",
			src: []interface{}{
				S{1, "one"},
				S{2, "two"},
				S{3, "three"},
			},
			wantErr: true,
		},
		{
			desc:    "slice length is 0, return error",
			src:     []interface{}{},
			wantErr: true,
		},
	}

	// Use the same keys and expected entities for all tests.
	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}
	want := []*pb.Mutation{
		{Operation: &pb.Mutation_Upsert{
			Upsert: &pb.Entity{
				Key: keyToProto(keys[0]),
				Properties: map[string]*pb.Value{
					"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
					"B": {ValueType: &pb.Value_StringValue{StringValue: "one"}},
				},
			}}},
		{Operation: &pb.Mutation_Upsert{
			Upsert: &pb.Entity{
				Key: keyToProto(keys[1]),
				Properties: map[string]*pb.Value{
					"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					"B": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
				},
			}}},
	}

	for _, tt := range testCases {
		// Set up a fake client which captures upserts.
		var got []*pb.Mutation
		client := &Client{
			client: &fakeClient{
				commitFn: func(req *pb.CommitRequest) (*pb.CommitResponse, error) {
					got = req.Mutations
					return &pb.CommitResponse{}, nil
				},
			},
		}

		_, err := client.PutMulti(ctx, keys, tt.src)
		if err != nil {
			if !tt.wantErr {
				t.Errorf("%s: error %v", tt.desc, err)
			}
			continue
		}
		if tt.wantErr {
			t.Errorf("%s: wanted error, but none returned", tt.desc)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s: got %d entities, want %d", tt.desc, len(got), len(want))
			continue
		}
		for i, e := range got {
			if !proto.Equal(e, want[i]) {
				t.Logf("%s: entity %d doesn't match\ngot:  %v\nwant: %v", tt.desc, i, e, want[i])
			}
		}
	}
}

func TestGetWithReadTime(t *testing.T) {
	type ent struct {
		A int
	}
	tm := time.Now()
	k := NameKey("testKind", "testReadTime", nil)
	e := &pb.Entity{
		Key: keyToProto(k),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
		},
	}

	client, srv, cleanup := newMock(t)
	defer cleanup()

	srv.addRPC(&pb.LookupRequest{
		ProjectId:  "projectID",
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(k),
		},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_ReadTime{
				ReadTime: &timestamppb.Timestamp{Seconds: tm.Unix()},
			},
		},
	}, &pb.LookupResponse{
		Found: []*pb.EntityResult{
			{
				Entity:  e,
				Version: 1,
			},
		},
	})
	ctx := context.Background()
	client.WithReadOptions(ReadTime(tm))
	dst := &ent{}
	err := client.Get(ctx, k, dst)
	if err != nil {
		t.Fatalf("Get() with ReadTime failed: %v\n", err)
	}
}

func TestGetMultiWithReadTime(t *testing.T) {
	type ent struct {
		A int
	}

	tm := time.Now()
	k := []*Key{
		NameKey("testKind", "testReadTime", nil),
		NameKey("testKind", "testReadTime2", nil),
	}

	e := &pb.Entity{
		Key: keyToProto(k[0]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
		},
	}
	e2 := &pb.Entity{
		Key: keyToProto(k[1]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
		},
	}

	client, srv, cleanup := newMock(t)
	defer cleanup()

	srv.addRPC(&pb.LookupRequest{
		ProjectId:  "projectID",
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(k[0]),
			keyToProto(k[1]),
		},
		ReadOptions: &pb.ReadOptions{
			ConsistencyType: &pb.ReadOptions_ReadTime{
				ReadTime: &timestamppb.Timestamp{Seconds: tm.Unix()},
			},
		},
	}, &pb.LookupResponse{
		Found: []*pb.EntityResult{
			{
				Entity:  e,
				Version: 1,
			}, {
				Entity:  e2,
				Version: 1,
			},
		},
	})

	ctx := context.Background()
	client.WithReadOptions(ReadTime(tm))
	dst := make([]*ent, len(k))
	err := client.GetMulti(ctx, k, dst)
	if err != nil {
		t.Fatalf("Get() with ReadTime failed: %v\n", err)
	}
}

func TestNoIndexOnSliceProperties(t *testing.T) {
	// Check that ExcludeFromIndexes is set on the inner elements,
	// rather than the top-level ArrayValue value.
	pl := PropertyList{
		Property{
			Name: "repeated",
			Value: []interface{}{
				123,
				false,
				"short",
				strings.Repeat("a", 1503),
			},
			NoIndex: true,
		},
	}
	key := NameKey("dummy", "dummy", nil)

	entity, err := saveEntity(key, &pl)
	if err != nil {
		t.Fatalf("saveEntity: %v", err)
	}

	want := &pb.Value{
		ValueType: &pb.Value_ArrayValue{ArrayValue: &pb.ArrayValue{Values: []*pb.Value{
			{ValueType: &pb.Value_IntegerValue{IntegerValue: 123}, ExcludeFromIndexes: true},
			{ValueType: &pb.Value_BooleanValue{BooleanValue: false}, ExcludeFromIndexes: true},
			{ValueType: &pb.Value_StringValue{StringValue: "short"}, ExcludeFromIndexes: true},
			{ValueType: &pb.Value_StringValue{StringValue: strings.Repeat("a", 1503)}, ExcludeFromIndexes: true},
		}}},
	}
	if got := entity.Properties["repeated"]; !proto.Equal(got, want) {
		t.Errorf("Entity proto differs\ngot:  %v\nwant: %v", got, want)
	}
}

type byName PropertyList

func (s byName) Len() int           { return len(s) }
func (s byName) Less(i, j int) bool { return s[i].Name < s[j].Name }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// sortPL sorts the property list by property name, and
// recursively sorts any nested property lists, or nested slices of
// property lists.
func sortPL(pl PropertyList) {
	sort.Stable(byName(pl))
	for _, p := range pl {
		switch p.Value.(type) {
		case *Entity:
			sortPL(p.Value.(*Entity).Properties)
		case []interface{}:
			for _, p2 := range p.Value.([]interface{}) {
				if nent, ok := p2.(*Entity); ok {
					sortPL(nent.Properties)
				}
			}
		}
	}
}

func TestValidGeoPoint(t *testing.T) {
	testCases := []struct {
		desc string
		pt   GeoPoint
		want bool
	}{
		{
			"valid",
			GeoPoint{67.21, 13.37},
			true,
		},
		{
			"high lat",
			GeoPoint{-90.01, 13.37},
			false,
		},
		{
			"low lat",
			GeoPoint{90.01, 13.37},
			false,
		},
		{
			"high lng",
			GeoPoint{67.21, 182},
			false,
		},
		{
			"low lng",
			GeoPoint{67.21, -181},
			false,
		},
	}

	for _, tc := range testCases {
		if got := tc.pt.Valid(); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.desc, got, tc.want)
		}
	}
}

func TestPutInvalidEntity(t *testing.T) {
	// Test that trying to put an invalid entity always returns the correct error
	// type.

	// Fake client that can pretend to start a transaction.
	fakeClient := &fakeDatastoreClient{
		beginTransaction: func(*pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
			return &pb.BeginTransactionResponse{
				Transaction: []byte("deadbeef"),
			}, nil
		},
	}
	client := &Client{
		client: fakeClient,
	}

	ctx := context.Background()
	key := IncompleteKey("kind", nil)

	_, err := client.Put(ctx, key, "invalid entity")
	if err != ErrInvalidEntityType {
		t.Errorf("client.Put returned err %v, want %v", err, ErrInvalidEntityType)
	}

	_, err = client.PutMulti(ctx, []*Key{key}, []interface{}{"invalid entity"})
	if me, ok := err.(MultiError); !ok {
		t.Errorf("client.PutMulti returned err %v, want MultiError type", err)
	} else if len(me) != 1 || me[0] != ErrInvalidEntityType {
		t.Errorf("client.PutMulti returned err %v, want MulitError{ErrInvalidEntityType}", err)
	}

	client.RunInTransaction(ctx, func(tx *Transaction) error {
		_, err := tx.Put(key, "invalid entity")
		if err != ErrInvalidEntityType {
			t.Errorf("tx.Put returned err %v, want %v", err, ErrInvalidEntityType)
		}

		_, err = tx.PutMulti([]*Key{key}, []interface{}{"invalid entity"})
		if me, ok := err.(MultiError); !ok {
			t.Errorf("tx.PutMulti returned err %v, want MultiError type", err)
		} else if len(me) != 1 || me[0] != ErrInvalidEntityType {
			t.Errorf("tx.PutMulti returned err %v, want MulitError{ErrInvalidEntityType}", err)
		}

		return errors.New("bang") // Return error: we don't actually want to commit.
	})
}

func TestDeferred(t *testing.T) {
	type Ent struct {
		A int
		B string
	}

	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}

	entity1 := &pb.Entity{
		Key: keyToProto(keys[0]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
			"B": {ValueType: &pb.Value_StringValue{StringValue: "one"}},
		},
	}
	entity2 := &pb.Entity{
		Key: keyToProto(keys[1]),
		Properties: map[string]*pb.Value{
			"A": {ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
			"B": {ValueType: &pb.Value_StringValue{StringValue: "two"}},
		},
	}

	// count keeps track of the number of times fakeClient.lookup has been
	// called.
	var count int
	// Fake client that will return Deferred keys in resp on the first call.
	fakeClient := &fakeDatastoreClient{
		lookup: func(*pb.LookupRequest) (*pb.LookupResponse, error) {
			count++
			// On the first call, we return deferred keys.
			if count == 1 {
				return &pb.LookupResponse{
					Found: []*pb.EntityResult{
						{
							Entity:  entity1,
							Version: 1,
						},
					},
					Deferred: []*pb.Key{
						keyToProto(keys[1]),
					},
				}, nil
			}

			// On the second call, we do not return any more deferred keys.
			return &pb.LookupResponse{
				Found: []*pb.EntityResult{
					{
						Entity:  entity2,
						Version: 1,
					},
				},
			}, nil
		},
	}
	client := &Client{
		client:       fakeClient,
		readSettings: &readSettings{},
	}

	ctx := context.Background()

	dst := make([]Ent, len(keys))
	err := client.GetMulti(ctx, keys, dst)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected client.lookup to be called 2 times. Got %d", count)
	}

	if len(dst) != 2 {
		t.Fatalf("expected 2 entities returned, got %d", len(dst))
	}

	for _, e := range dst {
		if e.A == 1 {
			if e.B != "one" {
				t.Fatalf("unexpected entity %+v", e)
			}
		} else if e.A == 2 {
			if e.B != "two" {
				t.Fatalf("unexpected entity %+v", e)
			}
		} else {
			t.Fatalf("unexpected entity %+v", e)
		}
	}

}

func TestDeferredMissing(t *testing.T) {
	type ent struct {
		A int
		B string
	}

	keys := []*Key{
		NameKey("testKind", "first", nil),
		NameKey("testKind", "second", nil),
	}

	entity1 := &pb.Entity{
		Key: keyToProto(keys[0]),
	}
	entity2 := &pb.Entity{
		Key: keyToProto(keys[1]),
	}

	client, srv, cleanup := newMock(t)
	defer cleanup()

	srv.addRPC(&pb.LookupRequest{
		ProjectId:  "projectID",
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(keys[0]),
			keyToProto(keys[1]),
		},
	}, &pb.LookupResponse{
		Missing: []*pb.EntityResult{
			{
				Entity:  entity1,
				Version: 1,
			},
		},
		Deferred: []*pb.Key{
			keyToProto(keys[1]),
		},
	})

	srv.addRPC(&pb.LookupRequest{
		ProjectId:  "projectID",
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(keys[1]),
		},
	}, &pb.LookupResponse{
		Missing: []*pb.EntityResult{
			{
				Entity:  entity2,
				Version: 1,
			},
		},
	})

	ctx := context.Background()

	dst := make([]ent, len(keys))
	err := client.GetMulti(ctx, keys, dst)
	errs, ok := err.(MultiError)
	if !ok {
		t.Fatalf("expected error returns to be MultiError; got %v", err)
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors returns, got %d", len(errs))
	}
	if errs[0] != ErrNoSuchEntity {
		t.Fatalf("expected error to be ErrNoSuchEntity; got %v", errs[0])
	}
	if errs[1] != ErrNoSuchEntity {
		t.Fatalf("expected error to be ErrNoSuchEntity; got %v", errs[1])
	}

	if len(dst) != 2 {
		t.Fatalf("expected 2 entities returned, got %d", len(dst))
	}

	for _, e := range dst {
		if e.A != 0 || e.B != "" {
			t.Fatalf("unexpected entity %+v", e)
		}
	}
}

func TestGetWithNilKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	err := client.Get(context.Background(), nil, []Property{})
	if err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
}

func TestGetMultiWithNilKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	dest := make([]PropertyList, 1)
	err := client.GetMulti(context.Background(), []*Key{nil}, dest)
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] != ErrInvalidKey {
		t.Fatalf("want MultiError{ErrInvalidKey}, got %v", me)
	}
}

func TestGetWithIncompleteKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	err := client.Get(context.Background(), &Key{Kind: "testKind"}, []Property{})
	if err == nil {
		t.Fatalf("want err, got nil")
	}
}

func TestGetMultiWithIncompleteKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	dest := make([]PropertyList, 1)
	err := client.GetMulti(context.Background(), []*Key{{Kind: "testKind"}}, dest)
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] == nil {
		t.Fatalf("want MultiError{err}, got %v", me)
	}
}

func TestDeleteWithNilKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	err := client.Delete(context.Background(), nil)
	if err != ErrInvalidKey {
		t.Fatalf("want ErrInvalidKey, got %v", err)
	}
}

func TestDeleteMultiWithNilKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	err := client.DeleteMulti(context.Background(), []*Key{nil})
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] != ErrInvalidKey {
		t.Fatalf("want MultiError{ErrInvalidKey}, got %v", me)
	}
}

func TestDeleteWithIncompleteKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	err := client.Delete(context.Background(), &Key{Kind: "testKind"})
	if err == nil {
		t.Fatalf("want err, got nil")
	}
}

func TestDeleteMultiWithIncompleteKey(t *testing.T) {
	client := &Client{readSettings: &readSettings{}}
	err := client.DeleteMulti(context.Background(), []*Key{{Kind: "testKind"}})
	if me, ok := err.(MultiError); !ok {
		t.Fatalf("want MultiError, got %v", err)
	} else if len(me) != 1 || me[0] == nil {
		t.Fatalf("want MultiError{err}, got %v", me)
	}
}

func TestBasicGet(t *testing.T) {
	cl, srv, cleanup := newMock(t)
	defer cleanup()

	type testEnt struct {
		A string
	}

	key := NameKey("foo", "bar", nil)

	srv.addRPC(&pb.LookupRequest{
		ProjectId:  "projectID",
		DatabaseId: "",
		Keys: []*pb.Key{
			keyToProto(key),
		},
	}, &pb.LookupResponse{
		Found: []*pb.EntityResult{
			{
				Entity: &pb.Entity{
					Key: keyToProto(key),
					Properties: map[string]*pb.Value{
						"A": {ValueType: &pb.Value_StringValue{StringValue: "one"}},
					},
				},
			},
		},
	})

	dst := &testEnt{}
	err := cl.Get(context.Background(), key, dst)
	if err != nil {
		t.Fatalf("datastore: test failed to get entity: %v", err)
	}
}
