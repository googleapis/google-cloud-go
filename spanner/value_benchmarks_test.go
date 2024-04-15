// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanner

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"cloud.google.com/go/civil"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/iterator"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

func BenchmarkEncodeIntArray(b *testing.B) {
	for _, s := range []struct {
		name string
		f    func(a []int) (*proto3.Value, *sppb.Type, error)
	}{
		{"Orig", encodeIntArrayOrig},
		{"Func", encodeIntArrayFunc},
		{"Reflect", encodeIntArrayReflect},
	} {
		b.Run(s.name, func(b *testing.B) {
			for _, size := range []int{1, 10, 100, 1000} {
				a := make([]int, size)
				b.Run(strconv.Itoa(size), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						s.f(a)
					}
				})
			}
		})
	}
}

func encodeIntArrayOrig(a []int) (*proto3.Value, *sppb.Type, error) {
	vs := make([]*proto3.Value, len(a))
	var err error
	for i := range a {
		vs[i], _, err = encodeValue(a[i])
		if err != nil {
			return nil, nil, err
		}
	}
	return listProto(vs...), listType(intType()), nil
}

func encodeIntArrayFunc(a []int) (*proto3.Value, *sppb.Type, error) {
	v, err := encodeArray(len(a), func(i int) interface{} { return a[i] })
	if err != nil {
		return nil, nil, err
	}
	return v, listType(intType()), nil
}

func encodeIntArrayReflect(a []int) (*proto3.Value, *sppb.Type, error) {
	v, err := encodeArrayReflect(a)
	if err != nil {
		return nil, nil, err
	}
	return v, listType(intType()), nil
}

func encodeArrayReflect(a interface{}) (*proto3.Value, error) {
	va := reflect.ValueOf(a)
	len := va.Len()
	vs := make([]*proto3.Value, len)
	var err error
	for i := 0; i < len; i++ {
		vs[i], _, err = encodeValue(va.Index(i).Interface())
		if err != nil {
			return nil, err
		}
	}
	return listProto(vs...), nil
}

func BenchmarkDecodeGeneric(b *testing.B) {
	v := stringProto("test")
	t := stringType()
	var g GenericColumnValue
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decodeValue(v, t, &g)
	}
}

func BenchmarkDecodeString(b *testing.B) {
	v := stringProto("test")
	t := stringType()
	var s string
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decodeValue(v, t, &s)
	}
}

func BenchmarkDecodeCustomString(b *testing.B) {
	v := stringProto("test")
	t := stringType()
	type CustomString string
	var s CustomString
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decodeValue(v, t, &s)
	}
}

func BenchmarkDecodeArray(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000} {
		vals := make([]*proto3.Value, size)
		for i := 0; i < size; i++ {
			vals[i] = dateProto(d1)
		}
		lv := &proto3.ListValue{Values: vals}
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for _, s := range []struct {
				name   string
				decode func(*proto3.ListValue)
			}{
				{"DateDirect", decodeArrayDateDirect},
				{"DateFunc", decodeArrayDateFunc},
				{"DateReflect", decodeArrayDateReflect},
				{"StringDecodeStringArray", decodeStringArrayWrap},
				{"StringDirect", decodeArrayStringDirect},
				{"StringFunc", decodeArrayStringFunc},
				{"StringReflect", decodeArrayStringReflect},
			} {
				b.Run(s.name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						s.decode(lv)
					}
				})
			}
		})

	}
}

func decodeArrayDateDirect(pb *proto3.ListValue) {
	a := make([]civil.Date, len(pb.Values))
	t := dateType()
	for i, v := range pb.Values {
		if err := decodeValue(v, t, &a[i]); err != nil {
			panic(err)
		}
	}
}

func decodeArrayDateFunc(pb *proto3.ListValue) {
	a := make([]civil.Date, len(pb.Values))
	if err := decodeArrayFunc(pb, "DATE", dateType(), func(i int) interface{} { return &a[i] }); err != nil {
		panic(err)
	}
}

func decodeArrayDateReflect(pb *proto3.ListValue) {
	var a []civil.Date
	if err := decodeArrayReflect(pb, "DATE", dateType(), &a); err != nil {
		panic(err)
	}
}

func decodeStringArrayWrap(pb *proto3.ListValue) {
	if _, err := decodeStringArray(pb); err != nil {
		panic(err)
	}
}

func decodeArrayStringDirect(pb *proto3.ListValue) {
	a := make([]string, len(pb.Values))
	t := stringType()
	for i, v := range pb.Values {
		if err := decodeValue(v, t, &a[i]); err != nil {
			panic(err)
		}
	}
}

func decodeArrayStringFunc(pb *proto3.ListValue) {
	a := make([]string, len(pb.Values))
	if err := decodeArrayFunc(pb, "STRING", stringType(), func(i int) interface{} { return &a[i] }); err != nil {
		panic(err)
	}
}

func decodeArrayStringReflect(pb *proto3.ListValue) {
	var a []string
	if err := decodeArrayReflect(pb, "STRING", stringType(), &a); err != nil {
		panic(err)
	}
}

func decodeArrayFunc(pb *proto3.ListValue, name string, typ *sppb.Type, elptr func(int) interface{}) error {
	if pb == nil {
		return errNilListValue(name)
	}
	for i, v := range pb.Values {
		if err := decodeValue(v, typ, elptr(i)); err != nil {
			return errDecodeArrayElement(i, v, name, err)
		}
	}
	return nil
}

func decodeArrayReflect(pb *proto3.ListValue, name string, typ *sppb.Type, aptr interface{}) error {
	if pb == nil {
		return errNilListValue(name)
	}
	av := reflect.ValueOf(aptr).Elem()
	av.Set(reflect.MakeSlice(av.Type(), len(pb.Values), len(pb.Values)))
	for i, v := range pb.Values {
		if err := decodeValue(v, typ, av.Index(i).Addr().Interface()); err != nil {
			av.Set(reflect.Zero(av.Type())) // reset slice to nil
			return errDecodeArrayElement(i, v, name, err)
		}
	}
	return nil
}

func BenchmarkScan(b *testing.B) {
	scanMethods := []string{"row.Column()", "row.ToStruct()", "row.SelectAll()"}
	for _, method := range scanMethods {
		for k := 0.; k <= 20; k++ {
			n := int(math.Pow(2, k))
			b.Run(fmt.Sprintf("%s/%d", method, n), func(b *testing.B) {
				b.StopTimer()
				var rows []struct {
					ID     int64
					Name   string
					Active bool
					City   string
					State  string
				}
				for i := 0; i < n; i++ {
					rows = append(rows, struct {
						ID     int64
						Name   string
						Active bool
						City   string
						State  string
					}{int64(i), fmt.Sprintf("name-%d", i), true, "city", "state"})
				}
				src := mockBenchmarkIterator(b, rows)
				for i := 0; i < b.N; i++ {
					it := *src
					var res []struct {
						ID     int64
						Name   string
						Active bool
						City   string
						State  string
					}
					b.StartTimer()
					switch method {
					case "row.SelectAll()":
						if err := SelectAll(&it, &res); err != nil {
							b.Fatal(err)
						}
						_ = res
						break
					default:
						for {
							row, err := it.Next()
							if err == iterator.Done {
								break
							} else if err != nil {
								b.Fatal(err)
							}
							var r struct {
								ID     int64
								Name   string
								Active bool
								City   string
								State  string
							}
							if method == "row.Column()" {
								err = row.Columns(&r.ID, &r.Name, &r.Active, &r.City, &r.State)
								if err != nil {
									b.Fatal(err)
								}
							} else {
								err = row.ToStruct(&r)
								if err != nil {
									b.Fatal(err)
								}
							}
							res = append(res, r)
						}
						it.Stop()
						_ = res
					}

				}
			})
		}
	}
}

func BenchmarkScan100RowsUsingSelectAll(b *testing.B) {
	var rows []struct {
		ID   int64
		Name string
	}
	for i := 0; i < 100; i++ {
		rows = append(rows, struct {
			ID   int64
			Name string
		}{int64(i), fmt.Sprintf("name-%d", i)})
	}
	src := mockBenchmarkIterator(b, rows)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		it := *src
		var res []struct {
			ID   int64
			Name string
		}
		if err := SelectAll(&it, &res); err != nil {
			b.Fatal(err)
		}
		_ = res
	}
}

func BenchmarkScan100RowsUsingToStruct(b *testing.B) {
	var rows []struct {
		ID   int64
		Name string
	}
	for i := 0; i < 100; i++ {
		rows = append(rows, struct {
			ID   int64
			Name string
		}{int64(i), fmt.Sprintf("name-%d", i)})
	}
	src := mockBenchmarkIterator(b, rows)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		it := *src
		var res []struct {
			ID   int64
			Name string
		}
		for {
			row, err := it.Next()
			if err == iterator.Done {
				break
			} else if err != nil {
				b.Fatal(err)
			}
			var r struct {
				ID   int64
				Name string
			}
			err = row.ToStruct(&r)
			if err != nil {
				b.Fatal(err)
			}
			res = append(res, r)
		}
		it.Stop()
		_ = res
	}
}

func BenchmarkScan100RowsUsingColumns(b *testing.B) {
	var rows []struct {
		ID   int64
		Name string
	}
	for i := 0; i < 100; i++ {
		rows = append(rows, struct {
			ID   int64
			Name string
		}{int64(i), fmt.Sprintf("name-%d", i)})
	}
	src := mockBenchmarkIterator(b, rows)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		it := *src
		var res []struct {
			ID   int64
			Name string
		}
		for {
			row, err := it.Next()
			if err == iterator.Done {
				break
			} else if err != nil {
				b.Fatal(err)
			}
			var r struct {
				ID   int64
				Name string
			}
			err = row.Columns(&r.ID, &r.Name)
			if err != nil {
				b.Fatal(err)
			}
			res = append(res, r)
		}
		it.Stop()
		_ = res
	}
}

func mockBenchmarkIterator[T any](t testing.TB, rows []T) *mockIteratorImpl {
	var v T
	var colNames []string
	numCols := reflect.TypeOf(v).NumField()
	for i := 0; i < numCols; i++ {
		f := reflect.TypeOf(v).Field(i)
		colNames = append(colNames, f.Name)
	}
	var srows []*Row
	for _, e := range rows {
		var vs []any
		for f := 0; f < numCols; f++ {
			v := reflect.ValueOf(e).Field(f).Interface()
			vs = append(vs, v)
		}
		row, err := NewRow(colNames, vs)
		if err != nil {
			t.Fatal(err)
		}
		srows = append(srows, row)
	}
	return &mockIteratorImpl{rows: srows}
}

type mockIteratorImpl struct {
	rows []*Row
}

func (i *mockIteratorImpl) Next() (*Row, error) {
	if len(i.rows) == 0 {
		return nil, iterator.Done
	}
	row := i.rows[0]
	i.rows = i.rows[1:]
	return row, nil
}

func (i *mockIteratorImpl) Stop() {
	i.rows = nil
}

func (i *mockIteratorImpl) Do(f func(*Row) error) error {
	defer i.Stop()
	for _, row := range i.rows {
		err := f(row)
		if err != nil {
			return err
		}
	}
	return nil
}
