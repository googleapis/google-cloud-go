package datastore

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/googlecloudplatform/gcloud-golang/datastore/pb"
)

type operator int

const (
	lessThan operator = iota
	lessEq
	equal
	greaterEq
	greaterThan
)

var operatorToProto = map[operator]*pb.PropertyFilter_Operator{
	lessThan:    pb.PropertyFilter_LESS_THAN.Enum(),
	lessEq:      pb.PropertyFilter_LESS_THAN_OR_EQUAL.Enum(),
	equal:       pb.PropertyFilter_EQUAL.Enum(),
	greaterEq:   pb.PropertyFilter_GREATER_THAN_OR_EQUAL.Enum(),
	greaterThan: pb.PropertyFilter_GREATER_THAN.Enum(),
}

// filter is a conditional filter on query results.
type filter struct {
	FieldName string
	Op        operator
	Value     interface{}
}

type sortDirection int

const (
	ascending sortDirection = iota
	descending
)

var sortDirectionToProto = map[sortDirection]*pb.PropertyOrder_Direction{
	ascending:  pb.PropertyOrder_ASCENDING.Enum(),
	descending: pb.PropertyOrder_ASCENDING.Enum(),
}

// order is a sort order on query results.
type order struct {
	FieldName string
	Direction sortDirection
}

var zeroCC []byte

// Cursor is an iterator's position. It can be converted to and from an opaque
// string. A cursor can be used from different HTTP requests, but only with a
// query with the same kind, ancestor, filter and order constraints.
type Cursor struct {
	cc []byte
}

// NewQuery creates a new Query for a specific entity kind.
//
// An empty kind means to return all entities, including entities created and
// managed by other App Engine features, and is called a kindless query.
// Kindless queries cannot include filters or sort orders on property values.
func (t *Transaction) NewQuery(kind string) *Query {
	return &Query{
		transaction: t,
		kind:        kind,
		limit:       -1,
	}
}

// Query represents a datastore query.
type Query struct {
	transaction *Transaction
	kind        string
	ancestor    *Key
	filter      []filter
	order       []order
	projection  []string

	distinct bool
	keysOnly bool
	eventual bool
	limit    int32
	offset   int32

	end []byte

	err error
}

func (q *Query) clone() *Query {
	x := *q
	// Copy the contents of the slice-typed fields to a new backing store.
	if len(q.filter) > 0 {
		x.filter = make([]filter, len(q.filter))
		copy(x.filter, q.filter)
	}
	if len(q.order) > 0 {
		x.order = make([]order, len(q.order))
		copy(x.order, q.order)
	}
	return &x
}

// Filter returns a derivative query with a field-based filter.
// The filterStr argument must be a field name followed by optional space,
// followed by an operator, one of ">", "<", ">=", "<=", or "=".
// Fields are compared against the provided value using the operator.
// Multiple filters are AND'ed together.
func (q *Query) Filter(filterStr string, value interface{}) *Query {
	q = q.clone()
	filterStr = strings.TrimSpace(filterStr)
	if len(filterStr) < 1 {
		q.err = errors.New("datastore: invalid filter: " + filterStr)
		return q
	}
	f := filter{
		FieldName: strings.TrimRight(filterStr, " ><=!"),
		Value:     value,
	}
	switch op := strings.TrimSpace(filterStr[len(f.FieldName):]); op {
	case "<=":
		f.Op = lessEq
	case ">=":
		f.Op = greaterEq
	case "<":
		f.Op = lessThan
	case ">":
		f.Op = greaterThan
	case "=":
		f.Op = equal
	default:
		q.err = fmt.Errorf("datastore: invalid operator %q in filter %q", op, filterStr)
		return q
	}
	q.filter = append(q.filter, f)
	return q
}

// Order returns a derivative query with a field-based sort order. Orders are
// applied in the order they are added. The default order is ascending; to sort
// in descending order prefix the fieldName with a minus sign (-).
func (q *Query) Order(fieldName string) *Query {
	q = q.clone()
	fieldName = strings.TrimSpace(fieldName)
	o := order{
		Direction: ascending,
		FieldName: fieldName,
	}
	if strings.HasPrefix(fieldName, "-") {
		o.Direction = descending
		o.FieldName = strings.TrimSpace(fieldName[1:])
	} else if strings.HasPrefix(fieldName, "+") {
		q.err = fmt.Errorf("datastore: invalid order: %q", fieldName)
		return q
	}
	if len(o.FieldName) == 0 {
		q.err = errors.New("datastore: empty order")
		return q
	}
	q.order = append(q.order, o)
	return q
}

// Project returns a derivative query that yields only the given fields. It
// cannot be used with KeysOnly.
func (q *Query) Project(fieldNames ...string) *Query {
	q = q.clone()
	q.projection = append([]string(nil), fieldNames...)
	return q
}

// Distinct returns a derivative query that yields de-duplicated entities with
// respect to the set of projected fields. It is only used for projection
// queries.
func (q *Query) Distinct() *Query {
	q = q.clone()
	q.distinct = true
	return q
}

// KeysOnly returns a derivative query that yields only keys, not keys and
// entities. It cannot be used with projection queries.
func (q *Query) KeysOnly() *Query {
	q = q.clone()
	q.keysOnly = true
	return q
}

// Limit returns a derivative query that has a limit on the number of results
// returned. A negative value means unlimited.
func (q *Query) Limit(limit int) *Query {
	q = q.clone()
	if limit < math.MinInt32 || limit > math.MaxInt32 {
		q.err = errors.New("datastore: query limit overflow")
		return q
	}
	q.limit = int32(limit)
	return q
}

// Offset returns a derivative query that has an offset of how many keys to
// skip over before returning results. A negative value is invalid.
func (q *Query) Offset(offset int) *Query {
	q = q.clone()
	if offset < 0 {
		q.err = errors.New("datastore: negative query offset")
		return q
	}
	if offset > math.MaxInt32 {
		q.err = errors.New("datastore: query offset overflow")
		return q
	}
	q.offset = int32(offset)
	return q
}

// Count returns the number of results for the query.
func (q *Query) Count() (int, error) {
	panic("not yet implemented")
}

// GetAll runs the query in the given context and returns all keys that match
// that query, as well as appending the values to dst.
//
// dst must have type *[]S or *[]*S or *[]P, for some struct type S or some non-
// interface, non-pointer type P such that P or *P implements PropertyLoadSaver.
//
// As a special case, *PropertyList is an invalid type for dst, even though a
// PropertyList is a slice of structs. It is treated as invalid to avoid being
// mistakenly passed when *[]PropertyList was intended.
//
// The keys returned by GetAll will be in a 1-1 correspondence with the entities
// added to dst.
//
// If q is a ``keys-only'' query, GetAll ignores dst and only returns the keys.
func (q *Query) GetAll(dst interface{}) ([]*Key, error) {
	panic("not yet implemented")
}

// Get gets the query results.
func (q *Query) Get() *Iterator {
	panic("not yet implemented")
}

// Iterator is the result of running a query.
type Iterator struct {
	err error
	// res is the result of the most recent RunQuery or Next API call.
	res pb.QueryResultBatch
	// i is how many elements of res.Result we have iterated over.
	i int
	// limit is the limit on the number of results this iterator should return.
	// A negative value means unlimited.
	limit int32
	// q is the original query which yielded this iterator.
	q *Query
	// prevCC is the compiled cursor that marks the end of the previous batch
	// of results.
	prevCC []byte
}

// Done is returned when a query iteration has completed.
var Done = errors.New("datastore: query has no more results")

// Next returns the key of the next result. When there are no more results,
// Done is returned as the error.
//
// If the query is not keys only and dst is non-nil, it also loads the entity
// stored for that key into the struct pointer or PropertyLoadSaver dst, with
// the same semantics and possible errors as for the Get function.
func (t *Iterator) Next(dst interface{}) (*Key, error) {
	panic("not implemented")
}

func (t *Iterator) HasNext() bool {
	panic("not implemented")
}

// Cursor returns a cursor for the iterator's current location.
func (t *Iterator) Cursor() (Cursor, error) {
	panic("not implementd")
}
