// Copyright 2015 Google LLC
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

package bigquery

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

func (fs *FieldSchema) GoString() string {
	if fs == nil {
		return "<nil>"
	}

	return fmt.Sprintf("{Name:%s Description:%s Repeated:%t Required:%t Type:%s Schema:%s}",
		fs.Name,
		fs.Description,
		fs.Repeated,
		fs.Required,
		fs.Type,
		fmt.Sprintf("%#v", fs.Schema),
	)
}

func bqTableFieldSchema(desc, name, typ, mode string, tags []string) *bq.TableFieldSchema {
	var policy *bq.TableFieldSchemaPolicyTags
	if tags != nil {
		policy = &bq.TableFieldSchemaPolicyTags{
			Names: tags,
		}
	}
	return &bq.TableFieldSchema{
		Description: desc,
		Name:        name,
		Mode:        mode,
		Type:        typ,
		PolicyTags:  policy,
	}
}

func fieldSchema(desc, name, typ string, repeated, required bool, tags []string) *FieldSchema {
	var policy *PolicyTagList
	if tags != nil {
		policy = &PolicyTagList{
			Names: tags,
		}
	}
	return &FieldSchema{
		Description: desc,
		Name:        name,
		Repeated:    repeated,
		Required:    required,
		Type:        FieldType(typ),
		PolicyTags:  policy,
	}
}

func TestRelaxSchema(t *testing.T) {
	testCases := []struct {
		in       Schema
		expected Schema
	}{
		{
			Schema{
				&FieldSchema{
					Description: "a relaxed schema",
					Required:    false,
					Type:        StringFieldType,
				},
			},
			Schema{
				&FieldSchema{
					Description: "a relaxed schema",
					Required:    false,
					Type:        StringFieldType,
				},
			},
		},
		{
			Schema{
				&FieldSchema{
					Description: "a required string",
					Required:    true,
					Type:        StringFieldType,
				},
				&FieldSchema{
					Description: "a required integer",
					Required:    true,
					Type:        IntegerFieldType,
				},
			},
			Schema{
				&FieldSchema{
					Description: "a required string",
					Required:    false,
					Type:        StringFieldType,
				},
				&FieldSchema{
					Description: "a required integer",
					Required:    false,
					Type:        IntegerFieldType,
				},
			},
		},
		{
			Schema{
				&FieldSchema{
					Description: "An outer schema wrapping a nested schema",
					Name:        "outer",
					Required:    true,
					Type:        RecordFieldType,
					Schema: Schema{
						{
							Description: "inner field",
							Name:        "inner",
							Type:        StringFieldType,
							Required:    true,
						},
					},
				},
			},
			Schema{
				&FieldSchema{
					Description: "An outer schema wrapping a nested schema",
					Name:        "outer",
					Required:    false,
					Type:        "RECORD",
					Schema: Schema{
						{
							Description: "inner field",
							Name:        "inner",
							Type:        "STRING",
							Required:    false,
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		converted := tc.in.Relax()
		if !testutil.Equal(converted, tc.expected) {
			t.Errorf("relaxing schema: got:\n%v\nwant:\n%v",
				pretty.Value(converted), pretty.Value(tc.expected))
		}
	}
}
func TestSchemaConversion(t *testing.T) {
	testCases := []struct {
		schema   Schema
		bqSchema *bq.TableSchema
	}{
		{
			// required
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "STRING", "REQUIRED", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "STRING", false, true, nil),
			},
		},
		{
			// repeated
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "STRING", "REPEATED", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "STRING", true, false, nil),
			},
		},
		{
			// nullable, string
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "STRING", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "STRING", false, false, nil),
			},
		},
		{
			// integer
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "INTEGER", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "INTEGER", false, false, nil),
			},
		},
		{
			// float
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "FLOAT", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "FLOAT", false, false, nil),
			},
		},
		{
			// boolean
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "BOOLEAN", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "BOOLEAN", false, false, nil),
			},
		},
		{
			// timestamp
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "name", "TIMESTAMP", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "name", "TIMESTAMP", false, false, nil),
			},
		},
		{
			// civil times
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "f1", "TIME", "", nil),
					bqTableFieldSchema("desc", "f2", "DATE", "", nil),
					bqTableFieldSchema("desc", "f3", "DATETIME", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "f1", "TIME", false, false, nil),
				fieldSchema("desc", "f2", "DATE", false, false, nil),
				fieldSchema("desc", "f3", "DATETIME", false, false, nil),
			},
		},
		{
			// numeric
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("desc", "n", "NUMERIC", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("desc", "n", "NUMERIC", false, false, nil),
			},
		},
		{
			// geography
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("geo", "g", "GEOGRAPHY", "", nil),
				},
			},
			schema: Schema{
				fieldSchema("geo", "g", "GEOGRAPHY", false, false, nil),
			},
		},
		{
			// constrained
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					{
						Name:      "foo",
						Type:      "STRING",
						MaxLength: 0,
						Precision: 0,
						Scale:     0,
					},
					{
						Name:      "bar",
						Type:      "STRING",
						MaxLength: 1,
						Precision: 2,
						Scale:     3,
					},
				}},
			schema: Schema{
				{Name: "foo",
					Type:      StringFieldType,
					MaxLength: 0,
					Precision: 0,
					Scale:     0,
				},
				{Name: "bar",
					Type:      StringFieldType,
					MaxLength: 1,
					Precision: 2,
					Scale:     3,
				},
			},
		},
		{
			// default values
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					{
						Name:                   "foo",
						Type:                   "STRING",
						DefaultValueExpression: "I_LOVE_FOO",
					},
					{
						Name:                   "bar",
						Type:                   "TIMESTAMP",
						DefaultValueExpression: "CURRENT_TIMESTAMP()",
					},
				}},
			schema: Schema{
				{
					Name:                   "foo",
					Type:                   StringFieldType,
					DefaultValueExpression: "I_LOVE_FOO",
				},
				{
					Name:                   "bar",
					Type:                   TimestampFieldType,
					DefaultValueExpression: "CURRENT_TIMESTAMP()",
				},
			},
		},
		{
			// collation values
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					{
						Name:      "name",
						Type:      "STRING",
						Collation: "und:ci",
					},
					{
						Name:      "another_name",
						Type:      "STRING",
						Collation: "",
					},
				}},
			schema: Schema{
				{
					Name:      "name",
					Type:      StringFieldType,
					Collation: "und:ci",
				},
				{
					Name:      "another_name",
					Type:      StringFieldType,
					Collation: "",
				},
			},
		},
		{
			// policy tags
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					bqTableFieldSchema("some pii", "restrictedfield", "STRING", "", []string{"tag1"}),
				},
			},
			schema: Schema{
				fieldSchema("some pii", "restrictedfield", "STRING", false, false, []string{"tag1"}),
			},
		},
		{
			// nested
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					{
						Description: "An outer schema wrapping a nested schema",
						Name:        "outer",
						Mode:        "REQUIRED",
						Type:        "RECORD",
						Fields: []*bq.TableFieldSchema{
							bqTableFieldSchema("inner field", "inner", "STRING", "", nil),
						},
					},
				},
			},
			schema: Schema{
				&FieldSchema{
					Description: "An outer schema wrapping a nested schema",
					Name:        "outer",
					Required:    true,
					Type:        "RECORD",
					Schema: Schema{
						{
							Description: "inner field",
							Name:        "inner",
							Type:        "STRING",
						},
					},
				},
			},
		},
		{
			// RANGE
			bqSchema: &bq.TableSchema{
				Fields: []*bq.TableFieldSchema{
					func() *bq.TableFieldSchema {
						f := bqTableFieldSchema("desc", "rt", "RANGE", "", nil)
						f.RangeElementType = &bq.TableFieldSchemaRangeElementType{
							Type: "DATE",
						}
						return f
					}(),
				},
			},
			schema: Schema{
				func() *FieldSchema {
					f := fieldSchema("desc", "rt", "RANGE", false, false, nil)
					f.RangeElementType = &RangeElementType{
						Type: DateFieldType,
					}
					return f
				}(),
			},
		},
	}
	for _, tc := range testCases {
		bqSchema := tc.schema.toBQ()
		if !testutil.Equal(bqSchema, tc.bqSchema) {
			t.Errorf("converting to TableSchema: got:\n%v\nwant:\n%v",
				pretty.Value(bqSchema), pretty.Value(tc.bqSchema))
		}
		schema := bqToSchema(tc.bqSchema)
		if !testutil.Equal(schema, tc.schema) {
			t.Errorf("converting to Schema: got:\n%v\nwant:\n%v", schema, tc.schema)
		}
	}
}

type allStrings struct {
	String    string
	ByteSlice []byte
}

type allSignedIntegers struct {
	Int64 int64
	Int32 int32
	Int16 int16
	Int8  int8
	Int   int
}

type allUnsignedIntegers struct {
	Uint32 uint32
	Uint16 uint16
	Uint8  uint8
}

type allFloat struct {
	Float64 float64
	Float32 float32
	// NOTE: Complex32 and Complex64 are unsupported by BigQuery
}

type allBoolean struct {
	Bool bool
}

type allTime struct {
	Timestamp time.Time
	Time      civil.Time
	Date      civil.Date
	DateTime  civil.DateTime
}

type allNumeric struct {
	Numeric *big.Rat
}

func reqField(name, typ string) *FieldSchema {
	return &FieldSchema{
		Name:     name,
		Type:     FieldType(typ),
		Required: true,
	}
}

func optField(name, typ string) *FieldSchema {
	return &FieldSchema{
		Name:     name,
		Type:     FieldType(typ),
		Required: false,
	}
}

func TestSimpleInference(t *testing.T) {
	testCases := []struct {
		in   interface{}
		want Schema
	}{
		{
			in: allSignedIntegers{},
			want: Schema{
				reqField("Int64", "INTEGER"),
				reqField("Int32", "INTEGER"),
				reqField("Int16", "INTEGER"),
				reqField("Int8", "INTEGER"),
				reqField("Int", "INTEGER"),
			},
		},
		{
			in: allUnsignedIntegers{},
			want: Schema{
				reqField("Uint32", "INTEGER"),
				reqField("Uint16", "INTEGER"),
				reqField("Uint8", "INTEGER"),
			},
		},
		{
			in: allFloat{},
			want: Schema{
				reqField("Float64", "FLOAT"),
				reqField("Float32", "FLOAT"),
			},
		},
		{
			in: allBoolean{},
			want: Schema{
				reqField("Bool", "BOOLEAN"),
			},
		},
		{
			in: &allBoolean{},
			want: Schema{
				reqField("Bool", "BOOLEAN"),
			},
		},
		{
			in: allTime{},
			want: Schema{
				reqField("Timestamp", "TIMESTAMP"),
				reqField("Time", "TIME"),
				reqField("Date", "DATE"),
				reqField("DateTime", "DATETIME"),
			},
		},
		{
			in: &allNumeric{},
			want: Schema{
				reqField("Numeric", "NUMERIC"),
			},
		},
		{
			in: allStrings{},
			want: Schema{
				reqField("String", "STRING"),
				reqField("ByteSlice", "BYTES"),
			},
		},
	}
	for _, tc := range testCases {
		got, err := InferSchema(tc.in)
		if err != nil {
			t.Fatalf("%T: error inferring TableSchema: %v", tc.in, err)
		}
		if !testutil.Equal(got, tc.want) {
			t.Errorf("%T: inferring TableSchema: got:\n%#v\nwant:\n%#v", tc.in,
				pretty.Value(got), pretty.Value(tc.want))
		}
	}
}

type containsNested struct {
	NotNested int
	Nested    struct {
		Inside int
	}
}

type containsDoubleNested struct {
	NotNested int
	Nested    struct {
		InsideNested struct {
			Inside int
		}
	}
}

type ptrNested struct {
	Ptr *struct{ Inside int }
}

type dup struct { // more than one field of the same struct type
	A, B allBoolean
}

func TestNestedInference(t *testing.T) {
	testCases := []struct {
		in   interface{}
		want Schema
	}{
		{
			in: containsNested{},
			want: Schema{
				reqField("NotNested", "INTEGER"),
				&FieldSchema{
					Name:     "Nested",
					Required: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("Inside", "INTEGER")},
				},
			},
		},
		{
			in: containsDoubleNested{},
			want: Schema{
				reqField("NotNested", "INTEGER"),
				&FieldSchema{
					Name:     "Nested",
					Required: true,
					Type:     "RECORD",
					Schema: Schema{
						{
							Name:     "InsideNested",
							Required: true,
							Type:     "RECORD",
							Schema:   Schema{reqField("Inside", "INTEGER")},
						},
					},
				},
			},
		},
		{
			in: ptrNested{},
			want: Schema{
				&FieldSchema{
					Name:     "Ptr",
					Required: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("Inside", "INTEGER")},
				},
			},
		},
		{
			in: dup{},
			want: Schema{
				&FieldSchema{
					Name:     "A",
					Required: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("Bool", "BOOLEAN")},
				},
				&FieldSchema{
					Name:     "B",
					Required: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("Bool", "BOOLEAN")},
				},
			},
		},
	}

	for _, tc := range testCases {
		got, err := InferSchema(tc.in)
		if err != nil {
			t.Fatalf("%T: error inferring TableSchema: %v", tc.in, err)
		}
		if !testutil.Equal(got, tc.want) {
			t.Errorf("%T: inferring TableSchema: got:\n%#v\nwant:\n%#v", tc.in,
				pretty.Value(got), pretty.Value(tc.want))
		}
	}
}

type repeated struct {
	NotRepeated       []byte
	RepeatedByteSlice [][]byte
	Slice             []int
	Array             [5]bool
}

type nestedRepeated struct {
	NotRepeated int
	Repeated    []struct {
		Inside int
	}
	RepeatedPtr []*struct{ Inside int }
}

func repField(name, typ string) *FieldSchema {
	return &FieldSchema{
		Name:     name,
		Type:     FieldType(typ),
		Repeated: true,
	}
}

func TestRepeatedInference(t *testing.T) {
	testCases := []struct {
		in   interface{}
		want Schema
	}{
		{
			in: repeated{},
			want: Schema{
				reqField("NotRepeated", "BYTES"),
				repField("RepeatedByteSlice", "BYTES"),
				repField("Slice", "INTEGER"),
				repField("Array", "BOOLEAN"),
			},
		},
		{
			in: nestedRepeated{},
			want: Schema{
				reqField("NotRepeated", "INTEGER"),
				{
					Name:     "Repeated",
					Repeated: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("Inside", "INTEGER")},
				},
				{
					Name:     "RepeatedPtr",
					Repeated: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("Inside", "INTEGER")},
				},
			},
		},
	}

	for i, tc := range testCases {
		got, err := InferSchema(tc.in)
		if err != nil {
			t.Fatalf("%d: error inferring TableSchema: %v", i, err)
		}
		if !testutil.Equal(got, tc.want) {
			t.Errorf("%d: inferring TableSchema: got:\n%#v\nwant:\n%#v", i,
				pretty.Value(got), pretty.Value(tc.want))
		}
	}
}

type allNulls struct {
	A NullInt64
	B NullFloat64
	C NullBool
	D NullString
	E NullTimestamp
	F NullTime
	G NullDate
	H NullDateTime
	I NullGeography
}

func TestNullInference(t *testing.T) {
	got, err := InferSchema(allNulls{})
	if err != nil {
		t.Fatal(err)
	}
	want := Schema{
		optField("A", "INTEGER"),
		optField("B", "FLOAT"),
		optField("C", "BOOLEAN"),
		optField("D", "STRING"),
		optField("E", "TIMESTAMP"),
		optField("F", "TIME"),
		optField("G", "DATE"),
		optField("H", "DATETIME"),
		optField("I", "GEOGRAPHY"),
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Error(diff)
	}
}

type Embedded struct {
	Embedded int
}

type embedded struct {
	Embedded2 int
}

type nestedEmbedded struct {
	Embedded
	embedded
}

func TestEmbeddedInference(t *testing.T) {
	got, err := InferSchema(nestedEmbedded{})
	if err != nil {
		t.Fatal(err)
	}
	want := Schema{
		reqField("Embedded", "INTEGER"),
		reqField("Embedded2", "INTEGER"),
	}
	if !testutil.Equal(got, want) {
		t.Errorf("got %v, want %v", pretty.Value(got), pretty.Value(want))
	}
}

func TestRecursiveInference(t *testing.T) {
	type List struct {
		Val  int
		Next *List
	}

	_, err := InferSchema(List{})
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

type withTags struct {
	NoTag         int
	ExcludeTag    int      `bigquery:"-"`
	SimpleTag     int      `bigquery:"simple_tag"`
	UnderscoreTag int      `bigquery:"_id"`
	MixedCase     int      `bigquery:"MIXEDcase"`
	Nullable      []byte   `bigquery:",nullable"`
	NullNumeric   *big.Rat `bigquery:",nullable"`
}

type withTagsNested struct {
	Nested          withTags `bigquery:"nested"`
	NestedAnonymous struct {
		ExcludeTag int `bigquery:"-"`
		Inside     int `bigquery:"inside"`
	} `bigquery:"anon"`
	PNested         *struct{ X int } // not nullable, for backwards compatibility
	PNestedNullable *struct{ X int } `bigquery:",nullable"`
}

type withTagsRepeated struct {
	Repeated          []withTags `bigquery:"repeated"`
	RepeatedAnonymous []struct {
		ExcludeTag int `bigquery:"-"`
		Inside     int `bigquery:"inside"`
	} `bigquery:"anon"`
}

type withTagsEmbedded struct {
	withTags
}

var withTagsSchema = Schema{
	reqField("NoTag", "INTEGER"),
	reqField("simple_tag", "INTEGER"),
	reqField("_id", "INTEGER"),
	reqField("MIXEDcase", "INTEGER"),
	optField("Nullable", "BYTES"),
	optField("NullNumeric", "NUMERIC"),
}

func TestTagInference(t *testing.T) {
	testCases := []struct {
		in   interface{}
		want Schema
	}{
		{
			in:   withTags{},
			want: withTagsSchema,
		},
		{
			in: withTagsNested{},
			want: Schema{
				&FieldSchema{
					Name:     "nested",
					Required: true,
					Type:     "RECORD",
					Schema:   withTagsSchema,
				},
				&FieldSchema{
					Name:     "anon",
					Required: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("inside", "INTEGER")},
				},
				&FieldSchema{
					Name:     "PNested",
					Required: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("X", "INTEGER")},
				},
				&FieldSchema{
					Name:     "PNestedNullable",
					Required: false,
					Type:     "RECORD",
					Schema:   Schema{reqField("X", "INTEGER")},
				},
			},
		},
		{
			in: withTagsRepeated{},
			want: Schema{
				&FieldSchema{
					Name:     "repeated",
					Repeated: true,
					Type:     "RECORD",
					Schema:   withTagsSchema,
				},
				&FieldSchema{
					Name:     "anon",
					Repeated: true,
					Type:     "RECORD",
					Schema:   Schema{reqField("inside", "INTEGER")},
				},
			},
		},
		{
			in:   withTagsEmbedded{},
			want: withTagsSchema,
		},
	}
	for i, tc := range testCases {
		got, err := InferSchema(tc.in)
		if err != nil {
			t.Fatalf("%d: error inferring TableSchema: %v", i, err)
		}
		if !testutil.Equal(got, tc.want) {
			t.Errorf("%d: inferring TableSchema: got:\n%#v\nwant:\n%#v", i,
				pretty.Value(got), pretty.Value(tc.want))
		}
	}
}

func TestTagInferenceErrors(t *testing.T) {
	testCases := []interface{}{
		struct {
			LongTag int `bigquery:"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxy"`
		}{},
		struct {
			UnsupporedStartChar int `bigquery:"øab"`
		}{},
		struct {
			UnsupportedEndChar int `bigquery:"abø"`
		}{},
		struct {
			UnsupportedMiddleChar int `bigquery:"aøb"`
		}{},
		struct {
			StartInt int `bigquery:"1abc"`
		}{},
		struct {
			Hyphens int `bigquery:"a-b"`
		}{},
	}
	for i, tc := range testCases {

		_, got := InferSchema(tc)
		if _, ok := got.(invalidFieldNameError); !ok {
			t.Errorf("%d: inferring TableSchema: got:\n%#v\nwant invalidFieldNameError", i, got)
		}
	}

	_, err := InferSchema(struct {
		X int `bigquery:",optional"`
	}{})
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestSchemaErrors(t *testing.T) {
	testCases := []struct {
		in   interface{}
		want interface{}
	}{
		{
			in:   []byte{},
			want: noStructError{},
		},
		{
			in:   new(int),
			want: noStructError{},
		},
		{
			in:   struct{ Uint uint }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Uint64 uint64 }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Uintptr uintptr }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Complex complex64 }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Map map[string]int }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Chan chan bool }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Ptr *int }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ Interface interface{} }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ MultiDimensional [][]int }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ MultiDimensional [][][]byte }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ SliceOfPointer []*int }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ SliceOfNull []NullInt64 }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ ChanSlice []chan bool }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ NestedChan struct{ Chan []chan bool } }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in: struct {
				X int `bigquery:",nullable"`
			}{},
			want: badNullableError{},
		},
		{
			in: struct {
				X bool `bigquery:",nullable"`
			}{},
			want: badNullableError{},
		},
		{
			in: struct {
				X struct{ N int } `bigquery:",nullable"`
			}{},
			want: badNullableError{},
		},
		{
			in: struct {
				X []int `bigquery:",nullable"`
			}{},
			want: badNullableError{},
		},
		{
			in:   struct{ X *[]byte }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ X *[]int }{},
			want: unsupportedFieldTypeError{},
		},
		{
			in:   struct{ X *int }{},
			want: unsupportedFieldTypeError{},
		},
	}
	for _, tc := range testCases {
		_, got := InferSchema(tc.in)
		if reflect.TypeOf(got) != reflect.TypeOf(tc.want) {
			t.Errorf("%#v: got:\n%#v\nwant type %T", tc.in, got, tc.want)
		}
	}
}

func TestHasRecursiveType(t *testing.T) {
	type (
		nonStruct int
		nonRec    struct{ A string }
		dup       struct{ A, B nonRec }
		rec       struct {
			A int
			B *rec
		}
		recUnexported struct {
			A int
		}
		hasRec struct {
			A int
			R *rec
		}
		recSlicePointer struct {
			A []*recSlicePointer
		}
	)
	for _, test := range []struct {
		in   interface{}
		want bool
	}{
		{nonStruct(0), false},
		{nonRec{}, false},
		{dup{}, false},
		{rec{}, true},
		{recUnexported{}, false},
		{hasRec{}, true},
		{&recSlicePointer{}, true},
	} {
		got, err := hasRecursiveType(reflect.TypeOf(test.in), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Errorf("%T: got %t, want %t", test.in, got, test.want)
		}
	}
}

func TestSchemaFromJSON(t *testing.T) {
	testCasesExpectingSuccess := []struct {
		bqSchemaJSON   []byte
		description    string
		expectedSchema Schema
	}{
		{
			description: "Flat table with a mixture of NULLABLE and REQUIRED fields",
			bqSchemaJSON: []byte(`
[
	{"name":"flat_string","type":"STRING","mode":"NULLABLE","description":"Flat nullable string"},
	{"name":"flat_bytes","type":"BYTES","mode":"REQUIRED","description":"Flat required BYTES"},
	{"name":"flat_integer","type":"INTEGER","mode":"NULLABLE","description":"Flat nullable INTEGER"},
	{"name":"flat_float","type":"FLOAT","mode":"REQUIRED","description":"Flat required FLOAT"},
	{"name":"flat_boolean","type":"BOOLEAN","mode":"NULLABLE","description":"Flat nullable BOOLEAN"},
	{"name":"flat_timestamp","type":"TIMESTAMP","mode":"REQUIRED","description":"Flat required TIMESTAMP"},
	{"name":"flat_date","type":"DATE","mode":"NULLABLE","description":"Flat required DATE"},
	{"name":"flat_time","type":"TIME","mode":"REQUIRED","description":"Flat nullable TIME"},
	{"name":"flat_datetime","type":"DATETIME","mode":"NULLABLE","description":"Flat required DATETIME"},
	{"name":"flat_numeric","type":"NUMERIC","mode":"REQUIRED","description":"Flat required NUMERIC"},
	{"name":"flat_bignumeric","type":"BIGNUMERIC","mode":"NULLABLE","description":"Flat nullable BIGNUMERIC"},
	{"name":"flat_geography","type":"GEOGRAPHY","mode":"REQUIRED","description":"Flat required GEOGRAPHY"},
	{"name":"aliased_integer","type":"INT64","mode":"REQUIRED","description":"Aliased required integer"},
	{"name":"aliased_boolean","type":"BOOL","mode":"NULLABLE","description":"Aliased nullable boolean"},
	{"name":"aliased_float","type":"FLOAT64","mode":"REQUIRED","description":"Aliased required float"},
	{"name":"aliased_record","type":"STRUCT","mode":"NULLABLE","description":"Aliased nullable record"},
	{"name":"aliased_bignumeric","type":"BIGDECIMAL","mode":"NULLABLE","description":"Aliased nullable bignumeric"},
	{"name":"flat_interval","type":"INTERVAL","mode":"NULLABLE","description":"Flat nullable interval"}
]`),
			expectedSchema: Schema{
				fieldSchema("Flat nullable string", "flat_string", "STRING", false, false, nil),
				fieldSchema("Flat required BYTES", "flat_bytes", "BYTES", false, true, nil),
				fieldSchema("Flat nullable INTEGER", "flat_integer", "INTEGER", false, false, nil),
				fieldSchema("Flat required FLOAT", "flat_float", "FLOAT", false, true, nil),
				fieldSchema("Flat nullable BOOLEAN", "flat_boolean", "BOOLEAN", false, false, nil),
				fieldSchema("Flat required TIMESTAMP", "flat_timestamp", "TIMESTAMP", false, true, nil),
				fieldSchema("Flat required DATE", "flat_date", "DATE", false, false, nil),
				fieldSchema("Flat nullable TIME", "flat_time", "TIME", false, true, nil),
				fieldSchema("Flat required DATETIME", "flat_datetime", "DATETIME", false, false, nil),
				fieldSchema("Flat required NUMERIC", "flat_numeric", "NUMERIC", false, true, nil),
				fieldSchema("Flat nullable BIGNUMERIC", "flat_bignumeric", "BIGNUMERIC", false, false, nil),
				fieldSchema("Flat required GEOGRAPHY", "flat_geography", "GEOGRAPHY", false, true, nil),
				fieldSchema("Aliased required integer", "aliased_integer", "INTEGER", false, true, nil),
				fieldSchema("Aliased nullable boolean", "aliased_boolean", "BOOLEAN", false, false, nil),
				fieldSchema("Aliased required float", "aliased_float", "FLOAT", false, true, nil),
				fieldSchema("Aliased nullable record", "aliased_record", "RECORD", false, false, nil),
				fieldSchema("Aliased nullable bignumeric", "aliased_bignumeric", "BIGNUMERIC", false, false, nil),
				fieldSchema("Flat nullable interval", "flat_interval", "INTERVAL", false, false, nil),
			},
		},
		{
			description: "Table with a nested RECORD",
			bqSchemaJSON: []byte(`
[
	{"name":"flat_string","type":"STRING","mode":"NULLABLE","description":"Flat nullable string"},
	{"name":"nested_record","type":"RECORD","mode":"NULLABLE","description":"Nested nullable RECORD","fields":[{"name":"record_field_1","type":"STRING","mode":"NULLABLE","description":"First nested record field"},{"name":"record_field_2","type":"INTEGER","mode":"REQUIRED","description":"Second nested record field"}]}
]`),
			expectedSchema: Schema{
				fieldSchema("Flat nullable string", "flat_string", "STRING", false, false, nil),
				&FieldSchema{
					Description: "Nested nullable RECORD",
					Name:        "nested_record",
					Required:    false,
					Type:        "RECORD",
					Schema: Schema{
						{
							Description: "First nested record field",
							Name:        "record_field_1",
							Required:    false,
							Type:        "STRING",
						},
						{
							Description: "Second nested record field",
							Name:        "record_field_2",
							Required:    true,
							Type:        "INTEGER",
						},
					},
				},
			},
		},
		{
			description: "Table with a repeated RECORD",
			bqSchemaJSON: []byte(`
[
	{"name":"flat_string","type":"STRING","mode":"NULLABLE","description":"Flat nullable string"},
	{"name":"nested_record","type":"RECORD","mode":"REPEATED","description":"Nested nullable RECORD","fields":[{"name":"record_field_1","type":"STRING","mode":"NULLABLE","description":"First nested record field"},{"name":"record_field_2","type":"INTEGER","mode":"REQUIRED","description":"Second nested record field"}]}
]`),
			expectedSchema: Schema{
				fieldSchema("Flat nullable string", "flat_string", "STRING", false, false, nil),
				&FieldSchema{
					Description: "Nested nullable RECORD",
					Name:        "nested_record",
					Repeated:    true,
					Required:    false,
					Type:        "RECORD",
					Schema: Schema{
						{
							Description: "First nested record field",
							Name:        "record_field_1",
							Required:    false,
							Type:        "STRING",
						},
						{
							Description: "Second nested record field",
							Name:        "record_field_2",
							Required:    true,
							Type:        "INTEGER",
						},
					},
				},
			},
		},
		{
			description: "Table with advanced parameters",
			bqSchemaJSON: []byte(`
[
	{"name":"strfield","type":"STRING","mode":"NULLABLE","description":"foo","maxLength":"100"},
	{"name":"numfield","type":"BIGNUMERIC","description":"bar","mode":"REPEATED","precision":"10","scale":"5","policyTags":{"names":["baz"]}}
]`),
			expectedSchema: Schema{
				&FieldSchema{
					Name:        "strfield",
					Description: "foo",
					MaxLength:   100,
					Type:        "STRING",
				},
				&FieldSchema{
					Name:        "numfield",
					Description: "bar",
					Repeated:    true,
					Type:        "BIGNUMERIC",
					Precision:   10,
					Scale:       5,
					PolicyTags: &PolicyTagList{
						Names: []string{"baz"},
					},
				},
			},
		},
	}
	for _, tc := range testCasesExpectingSuccess {
		convertedSchema, err := SchemaFromJSON(tc.bqSchemaJSON)
		if err != nil {
			t.Errorf("encountered an error when converting JSON table schema (%s): %v", tc.description, err)
			continue
		}
		if diff := testutil.Diff(convertedSchema, tc.expectedSchema); diff != "" {
			t.Errorf("%s: %s", tc.description, diff)
		}
	}

	testCasesExpectingFailure := []struct {
		bqSchemaJSON []byte
		description  string
	}{
		{
			description:  "Schema with invalid JSON",
			bqSchemaJSON: []byte(`This is not JSON`),
		},
		{
			description:  "Schema with unknown field type",
			bqSchemaJSON: []byte(`[{"name":"strange_type","type":"STRANGE","description":"This type should not exist"}]`),
		},
		{
			description:  "Schema with zero length",
			bqSchemaJSON: []byte(``),
		},
	}
	for _, tc := range testCasesExpectingFailure {
		_, err := SchemaFromJSON(tc.bqSchemaJSON)
		if err == nil {
			t.Errorf("converting this schema should have returned an error (%s): %v", tc.description, err)
			continue
		}
	}
}

func TestSchemaToJSONFields(t *testing.T) {

	// cmp option for comparing byte arrays without caring about whitespace.
	// courtesy of https://github.com/google/go-cmp/issues/224
	normalizeJSON := cmp.FilterValues(func(x, y []byte) bool {
		return json.Valid(x) && json.Valid(y)
	}, cmp.Transformer("ParseJSON", func(in []byte) (out interface{}) {
		if err := json.Unmarshal(in, &out); err != nil {
			panic(err)
		}
		return out
	}))

	testCases := []struct {
		description  string
		inSchema     Schema
		expectedJSON []byte
	}{
		{
			description: "basic schema",
			inSchema: Schema{
				fieldSchema("foo", "strfield", "STRING", false, false, nil),
				fieldSchema("bar", "intfield", "INTEGER", false, true, nil),
				fieldSchema("baz", "bool_arr", "INTEGER", true, false, []string{"tag1"}),
			},
			expectedJSON: []byte(`[
 {
  "description": "foo",
  "name": "strfield",
  "type": "STRING"
 },
 {
  "description": "bar",
  "mode": "REQUIRED",
  "name": "intfield",
  "type": "INTEGER"
 },
 {
  "description": "baz",
  "mode": "REPEATED",
  "name": "bool_arr",
  "policyTags": {
    "names": [
	  "tag1"
	]
   },
  "type": "INTEGER"
 }
]`),
		},
	}
	for _, tc := range testCases {
		got, err := tc.inSchema.ToJSONFields()
		if err != nil {
			t.Errorf("%s: %v", tc.description, err)
		}

		if diff := cmp.Diff(got, tc.expectedJSON, normalizeJSON); diff != "" {
			t.Errorf("%s: %s", tc.description, diff)
		}
	}
}
