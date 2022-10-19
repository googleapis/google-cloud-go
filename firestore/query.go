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
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"time"

	"cloud.google.com/go/internal/btree"
	"cloud.google.com/go/internal/trace"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

// Query represents a Firestore query.
//
// Query values are immutable. Each Query method creates
// a new Query; it does not modify the old.
type Query struct {
	c                      *Client
	path                   string // path to query (collection)
	parentPath             string // path of the collection's parent (document)
	collectionID           string
	selection              []*pb.StructuredQuery_FieldReference
	filters                []*pb.StructuredQuery_Filter
	orders                 []order
	offset                 int32
	limit                  *wrappers.Int32Value
	limitToLast            bool
	startVals, endVals     []interface{}
	startDoc, endDoc       *DocumentSnapshot
	startBefore, endBefore bool
	err                    error

	// allDescendants indicates whether this query is for all collections
	// that match the ID under the specified parentPath.
	allDescendants bool

	// readOptions specifies constraints for reading results from the query
	// e.g. read time
	readSettings *readSettings
}

// DocumentID is the special field name representing the ID of a document
// in queries.
const DocumentID = "__name__"

// Select returns a new Query that specifies the paths
// to return from the result documents.
// Each path argument can be a single field or a dot-separated sequence of
// fields, and must not contain any of the runes "˜*/[]".
//
// An empty Select call will produce a query that returns only document IDs.
func (q Query) Select(paths ...string) Query {
	var fps []FieldPath
	for _, s := range paths {
		fp, err := parseDotSeparatedString(s)
		if err != nil {
			q.err = err
			return q
		}
		fps = append(fps, fp)
	}
	return q.SelectPaths(fps...)
}

// SelectPaths returns a new Query that specifies the field paths
// to return from the result documents.
//
// An empty SelectPaths call will produce a query that returns only document IDs.
func (q Query) SelectPaths(fieldPaths ...FieldPath) Query {

	if len(fieldPaths) == 0 {
		ref, err := fref(FieldPath{DocumentID})
		if err != nil {
			q.err = err
			return q
		}
		q.selection = []*pb.StructuredQuery_FieldReference{
			ref,
		}
	} else {
		q.selection = make([]*pb.StructuredQuery_FieldReference, len(fieldPaths))
		for i, fieldPath := range fieldPaths {
			ref, err := fref(fieldPath)
			if err != nil {
				q.err = err
				return q
			}
			q.selection[i] = ref
		}
	}
	return q
}

// Where returns a new Query that filters the set of results.
// A Query can have multiple filters.
// The path argument can be a single field or a dot-separated sequence of
// fields, and must not contain any of the runes "˜*/[]".
// The op argument must be one of "==", "!=", "<", "<=", ">", ">=",
// "array-contains", "array-contains-any", "in" or "not-in".
func (q Query) Where(path, op string, value interface{}) Query {
	fp, err := parseDotSeparatedString(path)
	if err != nil {
		q.err = err
		return q
	}
	return q.WherePath(fp, op, value)
}

// WherePath returns a new Query that filters the set of results.
// A Query can have multiple filters.
// The op argument must be one of "==", "!=", "<", "<=", ">", ">=",
// "array-contains", "array-contains-any", "in" or "not-in".
func (q Query) WherePath(fp FieldPath, op string, value interface{}) Query {
	proto, err := filter{fp, op, value}.toProto()
	if err != nil {
		q.err = err
		return q
	}
	q.filters = append(append([]*pb.StructuredQuery_Filter(nil), q.filters...), proto)
	return q
}

// Direction is the sort direction for result ordering.
type Direction int32

const (
	// Asc sorts results from smallest to largest.
	Asc Direction = Direction(pb.StructuredQuery_ASCENDING)

	// Desc sorts results from largest to smallest.
	Desc Direction = Direction(pb.StructuredQuery_DESCENDING)
)

// OrderBy returns a new Query that specifies the order in which results are
// returned. A Query can have multiple OrderBy/OrderByPath specifications.
// OrderBy appends the specification to the list of existing ones.
//
// The path argument can be a single field or a dot-separated sequence of
// fields, and must not contain any of the runes "˜*/[]".
//
// To order by document name, use the special field path DocumentID.
func (q Query) OrderBy(path string, dir Direction) Query {
	fp, err := parseDotSeparatedString(path)
	if err != nil {
		q.err = err
		return q
	}
	q.orders = append(q.copyOrders(), order{fieldPath: fp, dir: dir})
	return q
}

// OrderByPath returns a new Query that specifies the order in which results are
// returned. A Query can have multiple OrderBy/OrderByPath specifications.
// OrderByPath appends the specification to the list of existing ones.
func (q Query) OrderByPath(fp FieldPath, dir Direction) Query {
	q.orders = append(q.copyOrders(), order{fieldPath: fp, dir: dir})
	return q
}

func (q *Query) copyOrders() []order {
	return append([]order(nil), q.orders...)
}

// Offset returns a new Query that specifies the number of initial results to skip.
// It must not be negative.
func (q Query) Offset(n int) Query {
	q.offset = trunc32(n)
	return q
}

// Limit returns a new Query that specifies the maximum number of first results
// to return. It must not be negative.
func (q Query) Limit(n int) Query {
	q.limit = &wrappers.Int32Value{Value: trunc32(n)}
	q.limitToLast = false
	return q
}

// LimitToLast returns a new Query that specifies the maximum number of last
// results to return. It must not be negative.
func (q Query) LimitToLast(n int) Query {
	q.limit = &wrappers.Int32Value{Value: trunc32(n)}
	q.limitToLast = true
	return q
}

// StartAt returns a new Query that specifies that results should start at
// the document with the given field values.
//
// StartAt may be called with a single DocumentSnapshot, representing an
// existing document within the query. The document must be a direct child of
// the location being queried (not a parent document, or document in a
// different collection, or a grandchild document, for example).
//
// Otherwise, StartAt should be called with one field value for each OrderBy clause,
// in the order that they appear. For example, in
//
//	q.OrderBy("X", Asc).OrderBy("Y", Desc).StartAt(1, 2)
//
// results will begin at the first document where X = 1 and Y = 2.
//
// If an OrderBy call uses the special DocumentID field path, the corresponding value
// should be the document ID relative to the query's collection. For example, to
// start at the document "NewYork" in the "States" collection, write
//
//	client.Collection("States").OrderBy(DocumentID, firestore.Asc).StartAt("NewYork")
//
// Calling StartAt overrides a previous call to StartAt or StartAfter.
func (q Query) StartAt(docSnapshotOrFieldValues ...interface{}) Query {
	q.startBefore = true
	q.startVals, q.startDoc, q.err = q.processCursorArg("StartAt", docSnapshotOrFieldValues)
	return q
}

// StartAfter returns a new Query that specifies that results should start just after
// the document with the given field values. See Query.StartAt for more information.
//
// Calling StartAfter overrides a previous call to StartAt or StartAfter.
func (q Query) StartAfter(docSnapshotOrFieldValues ...interface{}) Query {
	q.startBefore = false
	q.startVals, q.startDoc, q.err = q.processCursorArg("StartAfter", docSnapshotOrFieldValues)
	return q
}

// EndAt returns a new Query that specifies that results should end at the
// document with the given field values. See Query.StartAt for more information.
//
// Calling EndAt overrides a previous call to EndAt or EndBefore.
func (q Query) EndAt(docSnapshotOrFieldValues ...interface{}) Query {
	q.endBefore = false
	q.endVals, q.endDoc, q.err = q.processCursorArg("EndAt", docSnapshotOrFieldValues)
	return q
}

// EndBefore returns a new Query that specifies that results should end just before
// the document with the given field values. See Query.StartAt for more information.
//
// Calling EndBefore overrides a previous call to EndAt or EndBefore.
func (q Query) EndBefore(docSnapshotOrFieldValues ...interface{}) Query {
	q.endBefore = true
	q.endVals, q.endDoc, q.err = q.processCursorArg("EndBefore", docSnapshotOrFieldValues)
	return q
}

func (q *Query) processCursorArg(name string, docSnapshotOrFieldValues []interface{}) ([]interface{}, *DocumentSnapshot, error) {
	for _, e := range docSnapshotOrFieldValues {
		if ds, ok := e.(*DocumentSnapshot); ok {
			if len(docSnapshotOrFieldValues) == 1 {
				return nil, ds, nil
			}
			return nil, nil, fmt.Errorf("firestore: a document snapshot must be the only argument to %s", name)
		}
	}
	return docSnapshotOrFieldValues, nil, nil
}

func (q Query) query() *Query { return &q }

// Serialize creates a RunQueryRequest wire-format byte slice from a Query object.
// This can be used in combination with Deserialize to marshal Query objects.
// This could be useful, for instance, if executing a query formed in one
// process in another.
func (q Query) Serialize() ([]byte, error) {
	structuredQuery, err := q.toProto()
	if err != nil {
		return nil, err
	}

	p := &pb.RunQueryRequest{
		Parent:    q.parentPath,
		QueryType: &pb.RunQueryRequest_StructuredQuery{StructuredQuery: structuredQuery},
	}

	return proto.Marshal(p)
}

// Deserialize takes a slice of bytes holding the wire-format message of RunQueryRequest,
// the underlying proto message used by Queries. It then populates and returns a
// Query object that can be used to execut that Query.
func (q Query) Deserialize(bytes []byte) (Query, error) {
	runQueryRequest := pb.RunQueryRequest{}
	err := proto.Unmarshal(bytes, &runQueryRequest)
	if err != nil {
		q.err = err
		return q, err
	}
	return q.fromProto(&runQueryRequest)
}

// NewAggregationQuery returns an AggregationQuery with this query as its
// base query.
func (q *Query) NewAggregationQuery() *AggregationQuery {
	return &AggregationQuery{
		query: q,
	}
}

// fromProto creates a new Query object from a RunQueryRequest. This can be used
// in combination with ToProto to serialize Query objects. This could be useful,
// for instance, if executing a query formed in one process in another.
func (q Query) fromProto(pbQuery *pb.RunQueryRequest) (Query, error) {
	// Ensure we are starting from an empty query, but with this client.
	q = Query{c: q.c}

	pbq := pbQuery.GetStructuredQuery()
	if from := pbq.GetFrom(); len(from) > 0 {
		if len(from) > 1 {
			err := errors.New("can only deserialize query with exactly one collection selector")
			q.err = err
			return q, err
		}

		// collectionID           string
		q.collectionID = from[0].CollectionId
		// allDescendants indicates whether this query is for all collections
		// that match the ID under the specified parentPath.
		q.allDescendants = from[0].AllDescendants
	}

	// 	path                   string // path to query (collection)
	// 	parentPath             string // path of the collection's parent (document)
	parent := pbQuery.GetParent()
	q.parentPath = parent
	q.path = parent + "/" + q.collectionID

	// 	startVals, endVals     []interface{}
	// 	startDoc, endDoc       *DocumentSnapshot
	// 	startBefore, endBefore bool
	if startAt := pbq.GetStartAt(); startAt != nil {
		if startAt.GetBefore() {
			q.startBefore = true
		}
		for _, v := range startAt.GetValues() {
			c, err := createFromProtoValue(v, q.c)
			if err != nil {
				q.err = err
				return q, err
			}

			var newQ Query
			if startAt.GetBefore() {
				newQ = q.StartAt(c)
			} else {
				newQ = q.StartAfter(c)
			}

			q.startVals = append(q.startVals, newQ.startVals...)
		}
	}
	if endAt := pbq.GetEndAt(); endAt != nil {
		for _, v := range endAt.GetValues() {
			c, err := createFromProtoValue(v, q.c)

			if err != nil {
				q.err = err
				return q, err
			}

			var newQ Query
			if endAt.GetBefore() {
				newQ = q.EndBefore(c)
				q.endBefore = true
			} else {
				newQ = q.EndAt(c)
			}
			q.endVals = append(q.endVals, newQ.endVals...)

		}
	}

	// 	selection              []*pb.StructuredQuery_FieldReference
	if s := pbq.GetSelect(); s != nil {
		q.selection = s.GetFields()
	}

	// 	filters                []*pb.StructuredQuery_Filter
	if w := pbq.GetWhere(); w != nil {
		if cf := w.GetCompositeFilter(); cf != nil {
			q.filters = cf.GetFilters()
		} else {
			q.filters = []*pb.StructuredQuery_Filter{w}
		}
	}

	// 	orders                 []order
	if orderBy := pbq.GetOrderBy(); orderBy != nil {
		for _, v := range orderBy {
			fp := v.GetField()
			q.orders = append(q.orders, order{fieldReference: fp, dir: Direction(v.GetDirection())})
		}
	}

	// 	offset                 int32
	q.offset = pbq.GetOffset()

	// 	limit                  *wrappers.Int32Value
	if limit := pbq.GetLimit(); limit != nil {
		q.limit = limit
	}

	// NOTE: limit to last isn't part of the proto, this is a client-side concept
	// 	limitToLast            bool
	return q, q.err
}

func (q Query) toProto() (*pb.StructuredQuery, error) {
	if q.err != nil {
		return nil, q.err
	}
	if q.collectionID == "" {
		return nil, errors.New("firestore: query created without CollectionRef")
	}
	if q.startBefore {
		if len(q.startVals) == 0 && q.startDoc == nil {
			return nil, errors.New("firestore: StartAt/StartAfter must be called with at least one value")
		}
	}
	if q.endBefore {
		if len(q.endVals) == 0 && q.endDoc == nil {
			return nil, errors.New("firestore: EndAt/EndBefore must be called with at least one value")
		}
	}
	p := &pb.StructuredQuery{
		From: []*pb.StructuredQuery_CollectionSelector{{
			CollectionId:   q.collectionID,
			AllDescendants: q.allDescendants,
		}},
		Offset: q.offset,
		Limit:  q.limit,
	}
	if len(q.selection) > 0 {
		p.Select = &pb.StructuredQuery_Projection{}
		p.Select.Fields = q.selection
	}
	// If there is only filter, use it directly. Otherwise, construct
	// a CompositeFilter.
	if len(q.filters) == 1 {
		pf := q.filters[0]

		p.Where = pf
	} else if len(q.filters) > 1 {
		cf := &pb.StructuredQuery_CompositeFilter{
			Op: pb.StructuredQuery_CompositeFilter_AND,
		}
		p.Where = &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_CompositeFilter{cf},
		}
		cf.Filters = append(cf.Filters, q.filters...)
	}
	orders := q.orders
	if q.startDoc != nil || q.endDoc != nil {
		orders = q.adjustOrders()
	}
	for _, ord := range orders {
		po, err := ord.toProto()
		if err != nil {
			return nil, err
		}
		p.OrderBy = append(p.OrderBy, po)
	}

	cursor, err := q.toCursor(q.startVals, q.startDoc, q.startBefore, orders)
	if err != nil {
		return nil, err
	}
	p.StartAt = cursor
	cursor, err = q.toCursor(q.endVals, q.endDoc, q.endBefore, orders)
	if err != nil {
		return nil, err
	}
	p.EndAt = cursor
	return p, nil
}

// If there is a start/end that uses a Document Snapshot, we may need to adjust the OrderBy
// clauses that the user provided: we add OrderBy(__name__) if it isn't already present, and
// we make sure we don't invalidate the original query by adding an OrderBy for inequality filters.
func (q *Query) adjustOrders() []order {
	// If the user is already ordering by document ID, don't change anything.
	for _, ord := range q.orders {
		if ord.isDocumentID() {
			return q.orders
		}
	}
	// If there are OrderBy clauses, append an OrderBy(DocumentID), using the direction of the last OrderBy clause.
	if len(q.orders) > 0 {
		return append(q.copyOrders(), order{
			fieldPath: FieldPath{DocumentID},
			dir:       q.orders[len(q.orders)-1].dir,
		})
	}
	// If there are no OrderBy clauses but there is an inequality, add an OrderBy clause
	// for the field of the first inequality.
	var orders []order
	for _, f := range q.filters {
		if fieldFilter := f.GetFieldFilter(); fieldFilter != nil {
			if fieldFilter.Op != pb.StructuredQuery_FieldFilter_EQUAL {
				fp := f.GetFieldFilter().Field
				orders = []order{{fieldReference: fp, dir: Asc}}
				break
			}
		}
	}
	// Add an ascending OrderBy(DocumentID).
	return append(orders, order{fieldPath: FieldPath{DocumentID}, dir: Asc})
}

func (q *Query) toCursor(fieldValues []interface{}, ds *DocumentSnapshot, before bool, orders []order) (*pb.Cursor, error) {
	var vals []*pb.Value
	var err error
	if ds != nil {
		vals, err = q.docSnapshotToCursorValues(ds, orders)
	} else if len(fieldValues) != 0 {
		vals, err = q.fieldValuesToCursorValues(fieldValues)
	} else {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pb.Cursor{Values: vals, Before: before}, nil
}

// toPositionValues converts the field values to protos.
func (q *Query) fieldValuesToCursorValues(fieldValues []interface{}) ([]*pb.Value, error) {
	if len(fieldValues) != len(q.orders) {
		return nil, errors.New("firestore: number of field values in StartAt/StartAfter/EndAt/EndBefore does not match number of OrderBy fields")
	}
	vals := make([]*pb.Value, len(fieldValues))
	var err error
	for i, ord := range q.orders {
		fval := fieldValues[i]
		if ord.isDocumentID() {
			// TODO(jba): error if document ref does not belong to the right collection.

			switch docID := fval.(type) {
			case string:
				vals[i] = &pb.Value{ValueType: &pb.Value_ReferenceValue{q.path + "/" + docID}}
				continue
			case *DocumentRef:
				// DocumentRef can be transformed in usual way.
			default:
				return nil, fmt.Errorf("firestore: expected doc ID for DocumentID field, got %T", fval)
			}
		}

		var sawTransform bool
		vals[i], sawTransform, err = toProtoValue(reflect.ValueOf(fval))
		if err != nil {
			return nil, err
		}
		if sawTransform {
			return nil, errors.New("firestore: transforms disallowed in query value")
		}
	}
	return vals, nil
}

func (q *Query) docSnapshotToCursorValues(ds *DocumentSnapshot, orders []order) ([]*pb.Value, error) {
	vals := make([]*pb.Value, len(orders))
	for i, ord := range orders {
		if ord.isDocumentID() {
			dp, qp := ds.Ref.Parent.Path, q.path
			if !q.allDescendants && dp != qp {
				return nil, fmt.Errorf("firestore: document snapshot for %s passed to query on %s", dp, qp)
			}
			vals[i] = &pb.Value{ValueType: &pb.Value_ReferenceValue{ds.Ref.Path}}
		} else {
			var val *pb.Value
			var err error
			if len(ord.fieldPath) > 0 {
				val, err = valueAtPath(ord.fieldPath, ds.proto.Fields)
			} else {
				// parse the field reference field path so we can use it to look up
				fp, err := parseDotSeparatedString(ord.fieldReference.FieldPath)
				if err != nil {
					return nil, err
				}
				val, err = valueAtPath(fp, ds.proto.Fields)
			}
			if err != nil {
				return nil, err
			}
			vals[i] = val
		}
	}
	return vals, nil
}

// Returns a function that compares DocumentSnapshots according to q's ordering.
func (q Query) compareFunc() func(d1, d2 *DocumentSnapshot) (int, error) {
	// Add implicit sorting by name, using the last specified direction.
	lastDir := Asc
	if len(q.orders) > 0 {
		lastDir = q.orders[len(q.orders)-1].dir
	}
	orders := append(q.copyOrders(), order{fieldPath: []string{DocumentID}, dir: lastDir})
	return func(d1, d2 *DocumentSnapshot) (int, error) {
		for _, ord := range orders {
			var cmp int
			if len(ord.fieldPath) == 1 && ord.fieldPath[0] == DocumentID {
				cmp = compareReferences(d1.Ref.Path, d2.Ref.Path)
			} else {
				v1, err := valueAtPath(ord.fieldPath, d1.proto.Fields)
				if err != nil {
					return 0, err
				}
				v2, err := valueAtPath(ord.fieldPath, d2.proto.Fields)
				if err != nil {
					return 0, err
				}
				cmp = compareValues(v1, v2)
			}
			if cmp != 0 {
				if ord.dir == Desc {
					cmp = -cmp
				}
				return cmp, nil
			}
		}
		return 0, nil
	}
}

type filter struct {
	fieldPath FieldPath
	op        string
	value     interface{}
}

func (f filter) toProto() (*pb.StructuredQuery_Filter, error) {
	if err := f.fieldPath.validate(); err != nil {
		return nil, err
	}
	if uop, ok := unaryOpFor(f.value); ok {
		if f.op != "==" {
			return nil, fmt.Errorf("firestore: must use '==' when comparing %v", f.value)
		}
		ref, err := fref(f.fieldPath)
		if err != nil {
			return nil, err
		}
		return &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_UnaryFilter{
				UnaryFilter: &pb.StructuredQuery_UnaryFilter{
					OperandType: &pb.StructuredQuery_UnaryFilter_Field{
						Field: ref,
					},
					Op: uop,
				},
			},
		}, nil
	}
	var op pb.StructuredQuery_FieldFilter_Operator
	switch f.op {
	case "<":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN
	case "<=":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN_OR_EQUAL
	case ">":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN
	case ">=":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN_OR_EQUAL
	case "==":
		op = pb.StructuredQuery_FieldFilter_EQUAL
	case "!=":
		op = pb.StructuredQuery_FieldFilter_NOT_EQUAL
	case "in":
		op = pb.StructuredQuery_FieldFilter_IN
	case "not-in":
		op = pb.StructuredQuery_FieldFilter_NOT_IN
	case "array-contains":
		op = pb.StructuredQuery_FieldFilter_ARRAY_CONTAINS
	case "array-contains-any":
		op = pb.StructuredQuery_FieldFilter_ARRAY_CONTAINS_ANY
	default:
		return nil, fmt.Errorf("firestore: invalid operator %q", f.op)
	}
	val, sawTransform, err := toProtoValue(reflect.ValueOf(f.value))
	if err != nil {
		return nil, err
	}
	if sawTransform {
		return nil, errors.New("firestore: transforms disallowed in query value")
	}
	ref, err := fref(f.fieldPath)
	if err != nil {
		return nil, err
	}
	return &pb.StructuredQuery_Filter{
		FilterType: &pb.StructuredQuery_Filter_FieldFilter{
			FieldFilter: &pb.StructuredQuery_FieldFilter{
				Field: ref,
				Op:    op,
				Value: val,
			},
		},
	}, nil
}

func unaryOpFor(value interface{}) (pb.StructuredQuery_UnaryFilter_Operator, bool) {
	switch {
	case value == nil:
		return pb.StructuredQuery_UnaryFilter_IS_NULL, true
	case isNaN(value):
		return pb.StructuredQuery_UnaryFilter_IS_NAN, true
	default:
		return pb.StructuredQuery_UnaryFilter_OPERATOR_UNSPECIFIED, false
	}
}

func isNaN(x interface{}) bool {
	switch x := x.(type) {
	case float32:
		return math.IsNaN(float64(x))
	case float64:
		return math.IsNaN(x)
	default:
		return false
	}
}

type order struct {
	fieldPath      FieldPath
	fieldReference *pb.StructuredQuery_FieldReference
	dir            Direction
}

func (r order) isDocumentID() bool {
	if r.fieldReference != nil {
		return r.fieldReference.GetFieldPath() == DocumentID
	}
	return len(r.fieldPath) == 1 && r.fieldPath[0] == DocumentID
}

func (r order) toProto() (*pb.StructuredQuery_Order, error) {
	if r.fieldReference != nil {
		return &pb.StructuredQuery_Order{
			Field:     r.fieldReference,
			Direction: pb.StructuredQuery_Direction(r.dir),
		}, nil
	}

	field, err := fref(r.fieldPath)
	if err != nil {
		return nil, err
	}

	return &pb.StructuredQuery_Order{
		Field:     field,
		Direction: pb.StructuredQuery_Direction(r.dir),
	}, nil
}

func fref(fp FieldPath) (*pb.StructuredQuery_FieldReference, error) {
	err := fp.validate()
	if err != nil {
		return &pb.StructuredQuery_FieldReference{}, err
	}
	return &pb.StructuredQuery_FieldReference{FieldPath: fp.toServiceFieldPath()}, nil
}

func trunc32(i int) int32 {
	if i > math.MaxInt32 {
		i = math.MaxInt32
	}
	return int32(i)
}

// Documents returns an iterator over the query's resulting documents.
func (q Query) Documents(ctx context.Context) *DocumentIterator {
	return &DocumentIterator{
		iter: newQueryDocumentIterator(withResourceHeader(ctx, q.c.path()), &q, nil, q.readSettings), q: &q,
	}
}

// DocumentIterator is an iterator over documents returned by a query.
type DocumentIterator struct {
	iter docIterator
	err  error
	q    *Query
}

// Unexported interface so we can have two different kinds of DocumentIterator: one
// for straight queries, and one for query snapshots. We do it this way instead of
// making DocumentIterator an interface because in the client libraries, iterators are
// always concrete types, and the fact that this one has two different implementations
// is an internal detail.
type docIterator interface {
	next() (*DocumentSnapshot, error)
	stop()
}

// Next returns the next result. Its second return value is iterator.Done if there
// are no more results. Once Next returns Done, all subsequent calls will return
// Done.
func (it *DocumentIterator) Next() (*DocumentSnapshot, error) {
	if it.err != nil {
		return nil, it.err
	}
	if it.q.limitToLast {
		return nil, errors.New("firestore: queries that include limitToLast constraints cannot be streamed. Use DocumentIterator.GetAll() instead")
	}
	ds, err := it.iter.next()
	if err != nil {
		it.err = err
	}
	return ds, err
}

// Stop stops the iterator, freeing its resources.
// Always call Stop when you are done with a DocumentIterator.
// It is not safe to call Stop concurrently with Next.
func (it *DocumentIterator) Stop() {
	if it.iter != nil { // possible in error cases
		it.iter.stop()
	}
	if it.err == nil {
		it.err = iterator.Done
	}
}

// GetAll returns all the documents remaining from the iterator.
// It is not necessary to call Stop on the iterator after calling GetAll.
func (it *DocumentIterator) GetAll() ([]*DocumentSnapshot, error) {
	if it.err != nil {
		return nil, it.err
	}

	defer it.Stop()

	q := it.q
	limitedToLast := q.limitToLast
	if q.limitToLast {
		// Flip order statements before posting a request.
		for i := range q.orders {
			if q.orders[i].dir == Asc {
				q.orders[i].dir = Desc
			} else {
				q.orders[i].dir = Asc
			}
		}
		// Swap cursors.
		q.startVals, q.endVals = q.endVals, q.startVals
		q.startDoc, q.endDoc = q.endDoc, q.startDoc
		q.startBefore, q.endBefore = q.endBefore, q.startBefore

		q.limitToLast = false
	}
	var docs []*DocumentSnapshot
	for {
		doc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	if limitedToLast {
		// Flip docs order before return.
		for i, j := 0, len(docs)-1; i < j; {
			docs[i], docs[j] = docs[j], docs[i]
			i++
			j--
		}
	}
	return docs, nil
}

type queryDocumentIterator struct {
	ctx          context.Context
	cancel       func()
	q            *Query
	tid          []byte // transaction ID, if any
	streamClient pb.Firestore_RunQueryClient
	readSettings *readSettings // readOptions, if any
}

func newQueryDocumentIterator(ctx context.Context, q *Query, tid []byte, rs *readSettings) *queryDocumentIterator {
	ctx, cancel := context.WithCancel(ctx)
	return &queryDocumentIterator{
		ctx:          ctx,
		cancel:       cancel,
		q:            q,
		tid:          tid,
		readSettings: rs,
	}
}

func (it *queryDocumentIterator) next() (_ *DocumentSnapshot, err error) {
	client := it.q.c
	if it.streamClient == nil {
		it.ctx = trace.StartSpan(it.ctx, "cloud.google.com/go/firestore.Query.RunQuery")
		defer func() { trace.EndSpan(it.ctx, err) }()

		sq, err := it.q.toProto()
		if err != nil {
			return nil, err
		}
		req := &pb.RunQueryRequest{
			Parent:    it.q.parentPath,
			QueryType: &pb.RunQueryRequest_StructuredQuery{StructuredQuery: sq},
		}

		// Respect transactions first and read options (read time) second
		if rt, hasOpts := parseReadTime(client, it.readSettings); hasOpts {
			req.ConsistencySelector = &pb.RunQueryRequest_ReadTime{ReadTime: rt}
		}
		if it.tid != nil {
			req.ConsistencySelector = &pb.RunQueryRequest_Transaction{Transaction: it.tid}
		}
		it.streamClient, err = client.c.RunQuery(it.ctx, req)
		if err != nil {
			return nil, err
		}
	}
	var res *pb.RunQueryResponse
	for {
		res, err = it.streamClient.Recv()
		if err == io.EOF {
			return nil, iterator.Done
		}
		if err != nil {
			return nil, err
		}
		if res.Document != nil {
			break
		}
		// No document => partial progress; keep receiving.
	}
	docRef, err := pathToDoc(res.Document.Name, client)
	if err != nil {
		return nil, err
	}
	doc, err := newDocumentSnapshot(docRef, res.Document, client, res.ReadTime)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (it *queryDocumentIterator) stop() {
	it.cancel()
}

// Snapshots returns an iterator over snapshots of the query. Each time the query
// results change, a new snapshot will be generated.
func (q Query) Snapshots(ctx context.Context) *QuerySnapshotIterator {
	ws, err := newWatchStreamForQuery(ctx, q)
	if err != nil {
		return &QuerySnapshotIterator{err: err}
	}
	return &QuerySnapshotIterator{
		Query: q,
		ws:    ws,
	}
}

// QuerySnapshotIterator is an iterator over snapshots of a query.
// Call Next on the iterator to get a snapshot of the query's results each time they change.
// Call Stop on the iterator when done.
//
// For an example, see Query.Snapshots.
type QuerySnapshotIterator struct {
	// The Query used to construct this iterator.
	Query Query

	ws  *watchStream
	err error
}

// Next blocks until the query's results change, then returns a QuerySnapshot for
// the current results.
//
// Next is not expected to return iterator.Done unless it is called after Stop.
// Rarely, networking issues may also cause iterator.Done to be returned.
func (it *QuerySnapshotIterator) Next() (*QuerySnapshot, error) {
	if it.err != nil {
		return nil, it.err
	}
	btree, changes, readTime, err := it.ws.nextSnapshot()
	if err != nil {
		if err == io.EOF {
			err = iterator.Done
		}
		it.err = err
		return nil, it.err
	}
	return &QuerySnapshot{
		Documents: &DocumentIterator{
			iter: (*btreeDocumentIterator)(btree.BeforeIndex(0)), q: &it.Query,
		},
		Size:     btree.Len(),
		Changes:  changes,
		ReadTime: readTime,
	}, nil
}

// Stop stops receiving snapshots. You should always call Stop when you are done with
// a QuerySnapshotIterator, to free up resources. It is not safe to call Stop
// concurrently with Next.
func (it *QuerySnapshotIterator) Stop() {
	if it.ws != nil {
		it.ws.stop()
	}
}

// A QuerySnapshot is a snapshot of query results. It is returned by
// QuerySnapshotIterator.Next whenever the results of a query change.
type QuerySnapshot struct {
	// An iterator over the query results.
	// It is not necessary to call Stop on this iterator.
	Documents *DocumentIterator

	// The number of results in this snapshot.
	Size int

	// The changes since the previous snapshot.
	Changes []DocumentChange

	// The time at which this snapshot was obtained from Firestore.
	ReadTime time.Time
}

type btreeDocumentIterator btree.Iterator

func (it *btreeDocumentIterator) next() (*DocumentSnapshot, error) {
	if !(*btree.Iterator)(it).Next() {
		return nil, iterator.Done
	}
	return it.Key.(*DocumentSnapshot), nil
}

func (*btreeDocumentIterator) stop() {}

// WithReadOptions specifies constraints for accessing documents from the database,
// e.g. at what time snapshot to read the documents.
func (q *Query) WithReadOptions(opts ...ReadOption) *Query {
	for _, ro := range opts {
		ro.apply(q.readSettings)
	}
	return q
}

// AggregationQuery allows for generating aggregation results of an underlying
// basic query. A single AggregationQuery can contain multiple aggregations.
type AggregationQuery struct {
	// aggregateQueries contains all of the queries for this request.
	aggregateQueries []*pb.StructuredAggregationQuery_Aggregation
	// query contains a reference pointer to the underlying structured query.
	query *Query
}

// WithCount specifies that the aggregation query provide a count of results
// returned by the underlying Query.
func (a *AggregationQuery) WithCount(alias string) *AggregationQuery {
	aq := &pb.StructuredAggregationQuery_Aggregation{
		Alias:    alias,
		Operator: &pb.StructuredAggregationQuery_Aggregation_Count_{},
	}

	a.aggregateQueries = append(a.aggregateQueries, aq)

	return a
}

// Get retrieves the aggregation query results from the service.
func (a *AggregationQuery) Get(ctx context.Context) (AggregationResult, error) {

	client := a.query.c.c
	q, err := a.query.toProto()
	if err != nil {
		return nil, err
	}

	req := &pb.RunAggregationQueryRequest{
		Parent: a.query.parentPath,
		QueryType: &pb.RunAggregationQueryRequest_StructuredAggregationQuery{
			StructuredAggregationQuery: &pb.StructuredAggregationQuery{
				QueryType: &pb.StructuredAggregationQuery_StructuredQuery{
					StructuredQuery: q,
				},
				Aggregations: a.aggregateQueries,
			},
		},
	}
	ctx = withResourceHeader(ctx, a.query.c.path())
	stream, err := client.RunAggregationQuery(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := make(AggregationResult)

	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		f := res.Result.AggregateFields

		for k, v := range f {
			resp[k] = v
		}
	}
	return resp, nil
}

// AggregationResult contains the results of an aggregation query.
type AggregationResult map[string]interface{}
