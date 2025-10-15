/*
Copyright 2017 Google LLC

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

package spanner

import (
	"context"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

// keysetProto returns protobuf encoding of valid spanner.KeySet.
func keysetProto(t *testing.T, ks KeySet) *sppb.KeySet {
	k, err := ks.keySetProto()
	if err != nil {
		t.Fatalf("cannot convert keyset %v to protobuf: %v", ks, err)
	}
	return k
}

// Test encoding from spanner.Mutation to protobuf.
func TestMutationToProto(t *testing.T) {
	utc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("Could not load UTC: %v", err)
	}
	r, _ := (&big.Rat{}).SetString("3.14")

	for i, test := range []struct {
		m    *Mutation
		want *sppb.Mutation
	}{
		// Delete Mutation
		{
			&Mutation{opDelete, "t_foo", Key{"foo"}, nil, nil, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Delete_{
					Delete: &sppb.Mutation_Delete{
						Table:  "t_foo",
						KeySet: keysetProto(t, Key{"foo"}),
					},
				},
			},
		},
		// Insert Mutation
		{
			&Mutation{opInsert, "t_foo", KeySets(), []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Insert{
					Insert: &sppb.Mutation_Write{
						Table:   "t_foo",
						Columns: []string{"col1", "col2"},
						Values: []*proto3.ListValue{
							{
								Values: []*proto3.Value{intProto(1), intProto(2)},
							},
						},
					},
				},
			},
		},
		// InsertOrUpdate Mutation
		{
			&Mutation{opInsertOrUpdate, "t_foo", KeySets(), []string{"col1", "col2"}, []interface{}{1.0, 2.0}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_InsertOrUpdate{
					InsertOrUpdate: &sppb.Mutation_Write{
						Table:   "t_foo",
						Columns: []string{"col1", "col2"},
						Values: []*proto3.ListValue{
							{
								Values: []*proto3.Value{floatProto(1.0), floatProto(2.0)},
							},
						},
					},
				},
			},
		},
		// Replace Mutation
		{
			&Mutation{opReplace, "t_foo", KeySets(), []string{"col1", "col2"}, []interface{}{"one", 2.0}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Replace{
					Replace: &sppb.Mutation_Write{
						Table:   "t_foo",
						Columns: []string{"col1", "col2"},
						Values: []*proto3.ListValue{
							{
								Values: []*proto3.Value{stringProto("one"), floatProto(2.0)},
							},
						},
					},
				},
			},
		},
		// Update Mutation
		{
			&Mutation{opUpdate, "t_foo", KeySets(), []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Update{
					Update: &sppb.Mutation_Write{
						Table:   "t_foo",
						Columns: []string{"col1", "col2"},
						Values: []*proto3.ListValue{
							{
								Values: []*proto3.Value{stringProto("one"), nullProto()},
							},
						},
					},
				},
			},
		},
		// Mutation with all supported data types
		{
			&Mutation{
				opInsert,
				"t_foo",
				KeySets(),
				[]string{"colBool", "colInt64", "colFloat64", "colNumeric", "colString", "colBytes", "colDate", "colTimestamp"},
				[]interface{}{
					true,
					int64(100),
					float64(3.14),
					*r, // 3.14
					"one",
					[]byte{1, 2, 3},
					civil.Date{Year: 2020, Month: 12, Day: 2},
					time.Date(2020, time.December, 3, 8, 46, 58, 109, utc),
				},
				nil,
			},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Insert{
					Insert: &sppb.Mutation_Write{
						Table:   "t_foo",
						Columns: []string{"colBool", "colInt64", "colFloat64", "colNumeric", "colString", "colBytes", "colDate", "colTimestamp"},
						Values: []*proto3.ListValue{
							{
								Values: []*proto3.Value{
									boolProto(true),
									stringProto("100"),
									floatProto(3.14),
									stringProto("3.140000000"),
									stringProto("one"),
									bytesProto([]byte{1, 2, 3}),
									stringProto("2020-12-02"),
									stringProto("2020-12-03T08:46:58.000000109Z"),
								},
							},
						},
					},
				},
			},
		},
	} {
		if got, err := test.m.proto(); err != nil || !testEqual(got, test.want) {
			t.Errorf("%d:\n(%#v).proto() =\n     (%v, %v)\nwant (%v, nil)", i, test.m, got, err, test.want)
		}
		// Verify that wrapping the proto mutation as a Spanner mutation produces a mutation that returns the same
		// proto as the input argument.
		wrapped, err := WrapMutation(test.want)
		if err != nil {
			t.Errorf("WrapMutation failed for %v: %v", test.m, err)
		}
		if g, w := wrapped.op, test.m.op; g != w {
			t.Errorf("wrapped op mismatch\n Got: %v\n Want: %v", g, w)
		}
		if g, w := wrapped.table, test.m.table; g != w {
			t.Errorf("wrapped table mismatch\n Got: %v\n Want: %v", g, w)
		}
		proto, err := wrapped.proto()
		if err != nil {
			t.Errorf("converting wrapped mutation %v to proto failed: %v", wrapped, err)
		}
		// Note: We test for reference equality here, as the result should be the same instance as the wrapped mutation.
		if g, w := proto, test.want; g != w {
			t.Errorf("proto of wrapped mutation mismatch\n Got: %v\nWant: %v", g, w)
		}
	}
}

// mutationColumnSorter implements sort.Interface for sorting column-value pairs in a Mutation by column names.
type mutationColumnSorter struct {
	Mutation
}

// newMutationColumnSorter creates new instance of mutationColumnSorter by duplicating the input Mutation so that
// sorting won't change the input Mutation.
func newMutationColumnSorter(m *Mutation) *mutationColumnSorter {
	return &mutationColumnSorter{
		Mutation{
			m.op,
			m.table,
			m.keySet,
			append([]string(nil), m.columns...),
			append([]interface{}(nil), m.values...),
			nil,
		},
	}
}

// Len implements sort.Interface.Len.
func (ms *mutationColumnSorter) Len() int {
	return len(ms.columns)
}

// Swap implements sort.Interface.Swap.
func (ms *mutationColumnSorter) Swap(i, j int) {
	ms.columns[i], ms.columns[j] = ms.columns[j], ms.columns[i]
	ms.values[i], ms.values[j] = ms.values[j], ms.values[i]
}

// Less implements sort.Interface.Less.
func (ms *mutationColumnSorter) Less(i, j int) bool {
	return strings.Compare(ms.columns[i], ms.columns[j]) < 0
}

// mutationEqual returns true if two mutations in question are equal
// to each other.
func mutationEqual(t *testing.T, m1, m2 Mutation) bool {
	// Two mutations are considered to be equal even if their column values have different
	// orders.
	ms1 := newMutationColumnSorter(&m1)
	ms2 := newMutationColumnSorter(&m2)
	sort.Sort(ms1)
	sort.Sort(ms2)
	return testEqual(ms1, ms2)
}

// Test helper functions which help to generate spanner.Mutation.
func TestMutationHelpers(t *testing.T) {
	for _, test := range []struct {
		m    string
		got  *Mutation
		want *Mutation
	}{
		{
			"Insert",
			Insert("t_foo", []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}),
			&Mutation{opInsert, "t_foo", nil, []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}, nil},
		},
		{
			"InsertMap",
			InsertMap("t_foo", map[string]interface{}{"col1": int64(1), "col2": int64(2)}),
			&Mutation{opInsert, "t_foo", nil, []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}, nil},
		},
		{
			"InsertStruct",
			func() *Mutation {
				m, err := InsertStruct(
					"t_foo",
					struct {
						notCol bool
						Col1   int64 `spanner:"col1"`
						Col2   int64 `spanner:"col2"`
					}{false, int64(1), int64(2)},
				)
				if err != nil {
					t.Errorf("cannot convert struct into mutation: %v", err)
				}
				return m
			}(),
			&Mutation{opInsert, "t_foo", nil, []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}, nil},
		},
		{
			"Update",
			Update("t_foo", []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}),
			&Mutation{opUpdate, "t_foo", nil, []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}, nil},
		},
		{
			"UpdateMap",
			UpdateMap("t_foo", map[string]interface{}{"col1": "one", "col2": []byte(nil)}),
			&Mutation{opUpdate, "t_foo", nil, []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}, nil},
		},
		{
			"UpdateStruct",
			func() *Mutation {
				m, err := UpdateStruct(
					"t_foo",
					struct {
						Col1   string `spanner:"col1"`
						notCol int
						Col2   []byte `spanner:"col2"`
					}{"one", 1, nil},
				)
				if err != nil {
					t.Errorf("cannot convert struct into mutation: %v", err)
				}
				return m
			}(),
			&Mutation{opUpdate, "t_foo", nil, []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}, nil},
		},
		{
			"InsertOrUpdate",
			InsertOrUpdate("t_foo", []string{"col1", "col2"}, []interface{}{1.0, 2.0}),
			&Mutation{opInsertOrUpdate, "t_foo", nil, []string{"col1", "col2"}, []interface{}{1.0, 2.0}, nil},
		},
		{
			"InsertOrUpdateMap",
			InsertOrUpdateMap("t_foo", map[string]interface{}{"col1": 1.0, "col2": 2.0}),
			&Mutation{opInsertOrUpdate, "t_foo", nil, []string{"col1", "col2"}, []interface{}{1.0, 2.0}, nil},
		},
		{
			"InsertOrUpdateStruct",
			func() *Mutation {
				m, err := InsertOrUpdateStruct(
					"t_foo",
					struct {
						Col1   float64 `spanner:"col1"`
						Col2   float64 `spanner:"col2"`
						notCol float64
					}{1.0, 2.0, 3.0},
				)
				if err != nil {
					t.Errorf("cannot convert struct into mutation: %v", err)
				}
				return m
			}(),
			&Mutation{opInsertOrUpdate, "t_foo", nil, []string{"col1", "col2"}, []interface{}{1.0, 2.0}, nil},
		},
		{
			"Replace",
			Replace("t_foo", []string{"col1", "col2"}, []interface{}{"one", 2.0}),
			&Mutation{opReplace, "t_foo", nil, []string{"col1", "col2"}, []interface{}{"one", 2.0}, nil},
		},
		{
			"ReplaceMap",
			ReplaceMap("t_foo", map[string]interface{}{"col1": "one", "col2": 2.0}),
			&Mutation{opReplace, "t_foo", nil, []string{"col1", "col2"}, []interface{}{"one", 2.0}, nil},
		},
		{
			"ReplaceStruct",
			func() *Mutation {
				m, err := ReplaceStruct(
					"t_foo",
					struct {
						Col1   string  `spanner:"col1"`
						Col2   float64 `spanner:"col2"`
						notCol string
					}{"one", 2.0, "foo"},
				)
				if err != nil {
					t.Errorf("cannot convert struct into mutation: %v", err)
				}
				return m
			}(),
			&Mutation{opReplace, "t_foo", nil, []string{"col1", "col2"}, []interface{}{"one", 2.0}, nil},
		},
		{
			"Delete",
			Delete("t_foo", Key{"foo"}),
			&Mutation{opDelete, "t_foo", Key{"foo"}, nil, nil, nil},
		},
		{
			"DeleteRange",
			Delete("t_foo", KeyRange{Key{"bar"}, Key{"foo"}, ClosedClosed}),
			&Mutation{opDelete, "t_foo", KeyRange{Key{"bar"}, Key{"foo"}, ClosedClosed}, nil, nil, nil},
		},
	} {
		if !mutationEqual(t, *test.got, *test.want) {
			t.Errorf("%v: got Mutation %v, want %v", test.m, test.got, test.want)
		}
	}
}

// Test encoding non-struct types by using *Struct helpers.
func TestBadStructs(t *testing.T) {
	val := "i_am_not_a_struct"
	wantErr := errNotStruct(val)
	if _, gotErr := InsertStruct("t_test", val); !testEqual(gotErr, wantErr) {
		t.Errorf("InsertStruct(%q) returns error %v, want %v", val, gotErr, wantErr)
	}
	if _, gotErr := InsertOrUpdateStruct("t_test", val); !testEqual(gotErr, wantErr) {
		t.Errorf("InsertOrUpdateStruct(%q) returns error %v, want %v", val, gotErr, wantErr)
	}
	if _, gotErr := UpdateStruct("t_test", val); !testEqual(gotErr, wantErr) {
		t.Errorf("UpdateStruct(%q) returns error %v, want %v", val, gotErr, wantErr)
	}
	if _, gotErr := ReplaceStruct("t_test", val); !testEqual(gotErr, wantErr) {
		t.Errorf("ReplaceStruct(%q) returns error %v, want %v", val, gotErr, wantErr)
	}
}

func TestStructToMutationParams(t *testing.T) {
	// Tests cases not covered elsewhere.
	type S struct{ F interface{} }

	for _, test := range []struct {
		in       interface{}
		wantCols []string
		wantVals []interface{}
		wantErr  error
	}{
		{nil, nil, nil, errNotStruct(nil)},
		{3, nil, nil, errNotStruct(3)},
		{(*S)(nil), nil, nil, nil},
		{&S{F: 1}, []string{"F"}, []interface{}{1}, nil},
		{&S{F: CommitTimestamp}, []string{"F"}, []interface{}{CommitTimestamp}, nil},
	} {
		gotCols, gotVals, gotErr := structToMutationParams(test.in)
		if !testEqual(gotCols, test.wantCols) {
			t.Errorf("%#v: got cols %v, want %v", test.in, gotCols, test.wantCols)
		}
		if !testEqual(gotVals, test.wantVals) {
			t.Errorf("%#v: got vals %v, want %v", test.in, gotVals, test.wantVals)
		}
		if !testEqual(gotErr, test.wantErr) {
			t.Errorf("%#v: got err %v, want %v", test.in, gotErr, test.wantErr)
		}
	}
}

func TestStructToMutationParams_ReadOnly(t *testing.T) {
	t.Parallel()
	type ReadOnly struct {
		ID   int64
		Name string `spanner:"->"`
	}
	in := &ReadOnly{ID: 1, Name: "foo"}
	wantCols := []string{"ID"}
	wantVals := []interface{}{int64(1)}
	gotCols, gotVals, err := structToMutationParams(in)
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(gotCols, wantCols) {
		t.Errorf("got cols %v, want %v", gotCols, wantCols)
	}
	if !testEqual(gotVals, wantVals) {
		t.Errorf("got vals %v, want %v", gotVals, wantVals)
	}
}

func TestReadWrite_Generated(t *testing.T) {
	t.Parallel()
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	// The full name is generated by the server.
	server.TestSpanner.PutStatementResult(
		"SELECT Id, FirstName, LastName, FullName FROM Users WHERE Id = 1",
		&StatementResult{
			Type: StatementResultResultSet,
			ResultSet: &sppb.ResultSet{
				Metadata: &sppb.ResultSetMetadata{
					RowType: &sppb.StructType{
						Fields: []*sppb.StructType_Field{
							{Name: "Id", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
							{Name: "FirstName", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
							{Name: "LastName", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
							{Name: "FullName", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
						},
					},
				},
				Rows: []*proto3.ListValue{
					{
						Values: []*proto3.Value{
							intProto(1),
							stringProto("First"),
							stringProto("Last"),
							stringProto("First Last"),
						},
					},
				},
			},
		},
	)

	type User struct {
		ID        int64 `spanner:"Id"`
		FirstName string
		LastName  string
		FullName  string `spanner:"->"`
	}
	user := &User{
		ID:        1,
		FirstName: "First",
		LastName:  "Last",
	}
	m, err := InsertStruct("Users", user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), []*Mutation{m})
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the generated column 'FullName' was excluded from the write mutation.
	reqs := drainRequestsFromServer(server.TestSpanner)
	var commitReq *sppb.CommitRequest
	for _, r := range reqs {
		if c, ok := r.(*sppb.CommitRequest); ok {
			commitReq = c
		}
	}
	if commitReq == nil {
		t.Fatalf("no CommitRequest captured; got %v", reqs)
	}
	// Find the write mutation for Users.
	var write *sppb.Mutation_Write
	for _, mut := range commitReq.Mutations {
		if ins := mut.GetInsert(); ins != nil && ins.Table == "Users" {
			write = ins
			break
		}
	}
	if write == nil {
		t.Fatalf("no write mutation for table Users in CommitRequest: %v", commitReq.Mutations)
	}
	// Ensure FullName is not present in columns.
	for _, col := range write.Columns {
		if col == "FullName" {
			t.Fatalf("generated column FullName must be excluded from write.Columns: %v", write.Columns)
		}
	}
	wantCols := []string{"Id", "FirstName", "LastName"}
	if !reflect.DeepEqual(write.Columns, wantCols) {
		t.Fatalf("write.Columns mismatch\ngot  %v\nwant %v", write.Columns, wantCols)
	}
	if g, w := len(write.Values), 1; g != w {
		t.Fatalf("write.Values length mismatch: got %d, want 1", len(write.Values))
	}
	if g, w := len(write.Values[0].Values), len(wantCols); g != w {
		t.Fatalf("write.Values[0] length mismatch\n Got: %v\nWant: %v", g, w)
	}

	iter := client.Single().Query(context.Background(), NewStatement("SELECT Id, FirstName, LastName, FullName FROM Users WHERE Id = 1"))
	row, err := iter.Next()
	if err != nil {
		t.Fatal(err)
	}
	var got User
	if err := row.ToStruct(&got); err != nil {
		t.Fatal(err)
	}
	want := &User{
		ID:        1,
		FirstName: "First",
		LastName:  "Last",
		FullName:  "First Last",
	}
	if !testEqual(got, *want) {
		t.Errorf("got %v, want %v", got, *want)
	}
}

// Test encoding Mutation into proto.
func TestEncodeMutation(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutation  Mutation
		wantProto *sppb.Mutation
		wantErr   error
	}{
		{
			"OpDelete",
			Mutation{opDelete, "t_test", Key{1}, nil, nil, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Delete_{
					Delete: &sppb.Mutation_Delete{
						Table: "t_test",
						KeySet: &sppb.KeySet{
							Keys: []*proto3.ListValue{listValueProto(intProto(1))},
						},
					},
				},
			},
			nil,
		},
		{
			"OpDelete - Key error",
			Mutation{opDelete, "t_test", Key{struct{}{}}, nil, nil, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Delete_{
					Delete: &sppb.Mutation_Delete{
						Table:  "t_test",
						KeySet: &sppb.KeySet{},
					},
				},
			},
			errInvdKeyPartType(struct{}{}),
		},
		{
			"OpInsert",
			Mutation{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{"foo", 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Insert{
					Insert: &sppb.Mutation_Write{
						Table:   "t_test",
						Columns: []string{"key", "val"},
						Values:  []*proto3.ListValue{listValueProto(stringProto("foo"), intProto(1))},
					},
				},
			},
			nil,
		},
		{
			"OpInsert - Value Type Error",
			Mutation{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{struct{}{}, 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Insert{
					Insert: &sppb.Mutation_Write{},
				},
			},
			errEncoderUnsupportedType(struct{}{}),
		},
		{
			"OpInsertOrUpdate",
			Mutation{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo", 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_InsertOrUpdate{
					InsertOrUpdate: &sppb.Mutation_Write{
						Table:   "t_test",
						Columns: []string{"key", "val"},
						Values:  []*proto3.ListValue{listValueProto(stringProto("foo"), intProto(1))},
					},
				},
			},
			nil,
		},
		{
			"OpInsertOrUpdate - Value Type Error",
			Mutation{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{struct{}{}, 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_InsertOrUpdate{
					InsertOrUpdate: &sppb.Mutation_Write{},
				},
			},
			errEncoderUnsupportedType(struct{}{}),
		},
		{
			"OpReplace",
			Mutation{opReplace, "t_test", nil, []string{"key", "val"}, []interface{}{"foo", 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Replace{
					Replace: &sppb.Mutation_Write{
						Table:   "t_test",
						Columns: []string{"key", "val"},
						Values:  []*proto3.ListValue{listValueProto(stringProto("foo"), intProto(1))},
					},
				},
			},
			nil,
		},
		{
			"OpReplace - Value Type Error",
			Mutation{opReplace, "t_test", nil, []string{"key", "val"}, []interface{}{struct{}{}, 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Replace{
					Replace: &sppb.Mutation_Write{},
				},
			},
			errEncoderUnsupportedType(struct{}{}),
		},
		{
			"OpUpdate",
			Mutation{opUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo", 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Update{
					Update: &sppb.Mutation_Write{
						Table:   "t_test",
						Columns: []string{"key", "val"},
						Values:  []*proto3.ListValue{listValueProto(stringProto("foo"), intProto(1))},
					},
				},
			},
			nil,
		},
		{
			"OpUpdate - Value Type Error",
			Mutation{opUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{struct{}{}, 1}, nil},
			&sppb.Mutation{
				Operation: &sppb.Mutation_Update{
					Update: &sppb.Mutation_Write{},
				},
			},
			errEncoderUnsupportedType(struct{}{}),
		},
		{
			"OpKnown - Unknown Mutation Operation Code",
			Mutation{op(100), "t_test", nil, nil, nil, nil},
			&sppb.Mutation{},
			errInvdMutationOp(Mutation{op(100), "t_test", nil, nil, nil, nil}),
		},
	} {
		gotProto, gotErr := test.mutation.proto()
		if gotErr != nil {
			if !testEqual(gotErr, test.wantErr) {
				t.Errorf("%s: %v.proto() returns error %v, want %v", test.name, test.mutation, gotErr, test.wantErr)
			}
			continue
		}
		if !testEqual(gotProto, test.wantProto) {
			t.Errorf("%s: %v.proto() = (%v, nil), want (%v, nil)", test.name, test.mutation, gotProto, test.wantProto)
		}
	}
}

// Test Encoding an array of mutations.
func TestEncodeMutationArray(t *testing.T) {
	tests := []struct {
		name            string
		ms              []*Mutation
		want            []*sppb.Mutation
		wantMutationKey *sppb.Mutation
		wantErr         error
	}{
		// Test case for empty mutation list
		{
			name:            "Empty Mutation List",
			ms:              []*Mutation{},
			want:            []*sppb.Mutation{},
			wantMutationKey: nil,
			wantErr:         nil,
		},
		// Test case for only insert mutations
		{
			name: "Only Inserts",
			ms: []*Mutation{
				{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{"foo", 1}, nil},
				{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{"bar", 2}, nil},
				{opInsert, "t_test", nil, []string{"key", "val", "col3"}, []interface{}{"bar2", 3, 4}, nil},
			},
			want: []*sppb.Mutation{
				{
					Operation: &sppb.Mutation_Insert{
						Insert: &sppb.Mutation_Write{
							Table:   "t_test",
							Columns: []string{"key", "val"},
							Values: []*proto3.ListValue{
								listValueProto(stringProto("foo"), intProto(1)),
							},
						},
					},
				},
				{
					Operation: &sppb.Mutation_Insert{
						Insert: &sppb.Mutation_Write{
							Table:   "t_test",
							Columns: []string{"key", "val"},
							Values: []*proto3.ListValue{
								listValueProto(stringProto("bar"), intProto(2)),
							},
						},
					},
				},
				{
					Operation: &sppb.Mutation_Insert{
						Insert: &sppb.Mutation_Write{
							Table:   "t_test",
							Columns: []string{"key", "val", "col3"},
							Values: []*proto3.ListValue{
								listValueProto(stringProto("bar2"), intProto(3), intProto(4)),
							},
						},
					},
				},
			},
			wantMutationKey: &sppb.Mutation{
				Operation: &sppb.Mutation_Insert{
					Insert: &sppb.Mutation_Write{
						Table:   "t_test",
						Columns: []string{"key", "val", "col3"},
						Values: []*proto3.ListValue{
							listValueProto(stringProto("bar2"), intProto(3), intProto(4)),
						},
					},
				},
			},
			wantErr: nil,
		},
		// Test case for mixed operations
		{
			name: "Mixed Operations",
			ms: []*Mutation{
				{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{"foo", 1}, nil},
				{opUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"bar", 2}, nil},
			},
			want: []*sppb.Mutation{
				{
					Operation: &sppb.Mutation_Insert{
						Insert: &sppb.Mutation_Write{
							Table:   "t_test",
							Columns: []string{"key", "val"},
							Values: []*proto3.ListValue{
								listValueProto(stringProto("foo"), intProto(1)),
							},
						},
					},
				},
				{
					Operation: &sppb.Mutation_Update{
						Update: &sppb.Mutation_Write{
							Table:   "t_test",
							Columns: []string{"key", "val"},
							Values: []*proto3.ListValue{
								listValueProto(stringProto("bar"), intProto(2)),
							},
						},
					},
				},
			},
			wantMutationKey: &sppb.Mutation{
				Operation: &sppb.Mutation_Update{
					Update: &sppb.Mutation_Write{
						Table:   "t_test",
						Columns: []string{"key", "val"},
						Values: []*proto3.ListValue{
							listValueProto(stringProto("bar"), intProto(2)),
						},
					},
				},
			},
			wantErr: nil,
		},
		// Test case for error in mutation
		{
			name: "Error in Mutation",
			ms: []*Mutation{
				{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{struct{}{}, 1}, nil},
			},
			want:            []*sppb.Mutation{},
			wantMutationKey: nil,
			wantErr:         errEncoderUnsupportedType(struct{}{}),
		},
		// Test case for only delete mutations
		{
			name: "Only Deletes",
			ms: []*Mutation{
				{opDelete, "t_test", Key{"foo"}, nil, nil, nil},
				{opDelete, "t_test", Key{"bar"}, nil, nil, nil},
			},
			want: []*sppb.Mutation{
				{
					Operation: &sppb.Mutation_Delete_{
						Delete: &sppb.Mutation_Delete{
							Table: "t_test",
							KeySet: &sppb.KeySet{
								Keys: []*proto3.ListValue{
									listValueProto(stringProto("foo")),
								},
							},
						},
					},
				},
				{
					Operation: &sppb.Mutation_Delete_{
						Delete: &sppb.Mutation_Delete{
							Table: "t_test",
							KeySet: &sppb.KeySet{
								Keys: []*proto3.ListValue{
									listValueProto(stringProto("bar")),
								},
							},
						},
					},
				},
			},
			wantMutationKey: &sppb.Mutation{
				Operation: &sppb.Mutation_Delete_{
					Delete: &sppb.Mutation_Delete{
						Table: "t_test",
						KeySet: &sppb.KeySet{
							Keys: []*proto3.ListValue{
								listValueProto(stringProto("bar")),
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotProto, gotMutationKey, gotErr := mutationsProto(test.ms)
			if gotErr != nil {
				if !testEqual(gotErr, test.wantErr) {
					t.Errorf("mutationsProto(%v) returns error %v, want %v", test.ms, gotErr, test.wantErr)
				}
				return
			}
			if !testEqual(gotProto, test.want) {
				t.Errorf("mutationsProto(%v) = (%v, nil), want (%v, nil)", test.ms, gotProto, test.want)
			}
			if test.wantMutationKey != nil {
				if reflect.TypeOf(gotMutationKey.Operation) != reflect.TypeOf(test.wantMutationKey.Operation) {
					t.Errorf("mutationsProto(%v) returns mutation key %v, want %v", test.ms, gotMutationKey, test.wantMutationKey)
				}
			}
		})
	}
}

func TestEncodeMutationGroupArray(t *testing.T) {
	for _, test := range []struct {
		name    string
		mgs     []*MutationGroup
		want    []*sppb.BatchWriteRequest_MutationGroup
		wantErr error
	}{
		{
			"Multiple Mutations",
			[]*MutationGroup{
				{[]*Mutation{
					{opDelete, "t_test", Key{"bar"}, nil, nil, nil},
					{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", 1}, nil},
				}},
				{[]*Mutation{
					{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{"foo2", 1}, nil},
					{opUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo3", 1}, nil},
				}},
				{[]*Mutation{
					{opReplace, "t_test", nil, []string{"key", "val"}, []interface{}{"foo4", 1}, nil},
				}},
			},
			[]*sppb.BatchWriteRequest_MutationGroup{
				{Mutations: []*sppb.Mutation{
					{
						Operation: &sppb.Mutation_Delete_{
							Delete: &sppb.Mutation_Delete{
								Table: "t_test",
								KeySet: &sppb.KeySet{
									Keys: []*proto3.ListValue{listValueProto(stringProto("bar"))},
								},
							},
						},
					},
					{
						Operation: &sppb.Mutation_InsertOrUpdate{
							InsertOrUpdate: &sppb.Mutation_Write{
								Table:   "t_test",
								Columns: []string{"key", "val"},
								Values:  []*proto3.ListValue{listValueProto(stringProto("foo1"), intProto(1))},
							},
						},
					},
				}},
				{Mutations: []*sppb.Mutation{
					{
						Operation: &sppb.Mutation_Insert{
							Insert: &sppb.Mutation_Write{
								Table:   "t_test",
								Columns: []string{"key", "val"},
								Values:  []*proto3.ListValue{listValueProto(stringProto("foo2"), intProto(1))},
							},
						},
					},
					{
						Operation: &sppb.Mutation_Update{
							Update: &sppb.Mutation_Write{
								Table:   "t_test",
								Columns: []string{"key", "val"},
								Values:  []*proto3.ListValue{listValueProto(stringProto("foo3"), intProto(1))},
							},
						},
					},
				}},
				{Mutations: []*sppb.Mutation{
					{
						Operation: &sppb.Mutation_Replace{
							Replace: &sppb.Mutation_Write{
								Table:   "t_test",
								Columns: []string{"key", "val"},
								Values:  []*proto3.ListValue{listValueProto(stringProto("foo4"), intProto(1))},
							},
						},
					},
				}},
			},
			nil,
		},
		{
			"Multiple Mutations - Bad Mutation",
			[]*MutationGroup{
				{[]*Mutation{
					{opDelete, "t_test", Key{"bar"}, nil, nil, nil},
					{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo1", struct{}{}}, nil},
				}},
				{[]*Mutation{
					{opInsert, "t_test", nil, []string{"key", "val"}, []interface{}{"foo2", 1}, nil},
					{opUpdate, "t_test", nil, []string{"key", "val"}, []interface{}{"foo3", 1}, nil},
				}},
			},
			[]*sppb.BatchWriteRequest_MutationGroup{},
			errEncoderUnsupportedType(struct{}{}),
		},
	} {
		gotProto, gotErr := mutationGroupsProto(test.mgs)
		if gotErr != nil {
			if !testEqual(gotErr, test.wantErr) {
				t.Errorf("%v: mutationGroupsProto(%v) returns error %v, want %v", test.name, test.mgs, gotErr, test.wantErr)
			}
			continue
		}
		if !testEqual(gotProto, test.want) {
			t.Errorf("%v: mutationGroupsProto(%v) = (%v, nil), want (%v, nil)", test.name, test.mgs, gotProto, test.want)
		}
	}
}

func BenchmarkMutationsProto(b *testing.B) {
	type benchmarkCase struct {
		name      string
		mutations []*Mutation
	}
	benchmarkCases := []benchmarkCase{
		{
			name: "small number of mutations",
			mutations: []*Mutation{
				Insert("t_foo", []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}),
				Update("t_foo", []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}),
				InsertOrUpdate("t_foo", []string{"col1", "col2"}, []interface{}{1.0, 2.0}),
				Replace("t_foo", []string{"col1", "col2"}, []interface{}{"one", 2.0}),
				Delete("t_foo", Key{"foo"}),
			},
		},
		{
			name: "large number of mutations",
			mutations: func() []*Mutation {
				var mutations []*Mutation
				for i := 0; i < 20; i++ {
					mutations = append(mutations, Insert("t_foo", []string{"col1", "col2"}, []interface{}{int64(i), int64(i + 1)}))
					mutations = append(mutations, Update("t_foo", []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}))
					mutations = append(mutations, InsertOrUpdate("t_foo", []string{"col1", "col2"}, []interface{}{1.0, 2.0}))
					mutations = append(mutations, Replace("t_foo", []string{"col1", "col2"}, []interface{}{"one", 2.0}))
					mutations = append(mutations, Delete("t_foo", Key{i}))
				}
				return mutations
			}(),
		},
		{
			name: "mixed type of mutations",
			mutations: []*Mutation{
				Insert("t_foo", []string{"col1", "col2"}, []interface{}{int64(1), int64(2)}),
				Update("t_foo", []string{"col1", "col2"}, []interface{}{"one", []byte(nil)}),
				Delete("t_foo", Key{"foo"}),
				Insert("t_bar", []string{"col1"}, []interface{}{"bar"}),
			},
		},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, _ = mutationsProto(bc.mutations)
			}
		})
	}
}
