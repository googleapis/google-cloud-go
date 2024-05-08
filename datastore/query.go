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
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/internal/protostruct"
	"cloud.google.com/go/internal/trace"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

type operator string

const (
	lessThan    operator = "<"
	lessEq      operator = "<="
	equal       operator = "="
	greaterEq   operator = ">="
	greaterThan operator = ">"
	in          operator = "in"
	notIn       operator = "not-in"
	notEqual    operator = "!="

	keyFieldName = "__key__"
)

var stringToOperator = createStringToOperator()

func createStringToOperator() map[string]operator {
	strToOp := make(map[string]operator)
	for op := range operatorToProto {
		strToOp[string(op)] = op
	}
	return strToOp
}

var operatorToProto = map[operator]pb.PropertyFilter_Operator{
	lessThan:    pb.PropertyFilter_LESS_THAN,
	lessEq:      pb.PropertyFilter_LESS_THAN_OR_EQUAL,
	equal:       pb.PropertyFilter_EQUAL,
	greaterEq:   pb.PropertyFilter_GREATER_THAN_OR_EQUAL,
	greaterThan: pb.PropertyFilter_GREATER_THAN,
	in:          pb.PropertyFilter_IN,
	notIn:       pb.PropertyFilter_NOT_IN,
	notEqual:    pb.PropertyFilter_NOT_EQUAL,
}

type sortDirection bool

const (
	ascending  sortDirection = false
	descending sortDirection = true
)

var sortDirectionToProto = map[sortDirection]pb.PropertyOrder_Direction{
	ascending:  pb.PropertyOrder_ASCENDING,
	descending: pb.PropertyOrder_DESCENDING,
}

// order is a sort order on query results.
type order struct {
	FieldName string
	Direction sortDirection
}

// EntityFilter represents a datastore filter.
type EntityFilter interface {
	toValidFilter() (EntityFilter, error)
	toProto() (*pb.Filter, error)
}

// PropertyFilter represents field based filter.
//
// The operator parameter takes the following strings: ">", "<", ">=", "<=",
// "=", "!=", "in", and "not-in".
// Fields are compared against the provided value using the operator.
// Field names which contain spaces, quote marks, or operator characters
// should be passed as quoted Go string literals as returned by strconv.Quote
// or the fmt package's %q verb.
type PropertyFilter struct {
	FieldName string
	Operator  string
	Value     interface{}
}

func (pf PropertyFilter) toProto() (*pb.Filter, error) {

	if pf.FieldName == "" {
		return nil, errors.New("datastore: empty query filter field name")
	}
	v, err := interfaceToProto(reflect.ValueOf(pf.Value).Interface(), false)
	if err != nil {
		return nil, fmt.Errorf("datastore: bad query filter value type: %w", err)
	}

	op, isOp := stringToOperator[pf.Operator]
	if !isOp {
		return nil, fmt.Errorf("datastore: invalid operator %q in filter", pf.Operator)
	}

	opProto, ok := operatorToProto[op]
	if !ok {
		return nil, errors.New("datastore: unknown query filter operator")
	}
	xf := &pb.PropertyFilter{
		Op:       opProto,
		Property: &pb.PropertyReference{Name: pf.FieldName},
		Value:    v,
	}
	return &pb.Filter{
		FilterType: &pb.Filter_PropertyFilter{PropertyFilter: xf},
	}, nil
}

func (pf PropertyFilter) toValidFilter() (EntityFilter, error) {
	op := strings.TrimSpace(pf.Operator)
	_, isOp := stringToOperator[op]
	if !isOp {
		return nil, fmt.Errorf("datastore: invalid operator %q in filter", pf.Operator)
	}

	unquotedFieldName, err := unquote(pf.FieldName)
	if err != nil {
		return nil, fmt.Errorf("datastore: invalid syntax for quoted field name %q", pf.FieldName)
	}

	return PropertyFilter{Operator: op, FieldName: unquotedFieldName, Value: pf.Value}, nil
}

// CompositeFilter represents datastore composite filters.
type CompositeFilter interface {
	EntityFilter
	isCompositeFilter()
}

// OrFilter represents a union of two or more filters.
type OrFilter struct {
	Filters []EntityFilter
}

func (OrFilter) isCompositeFilter() {}

func (of OrFilter) toProto() (*pb.Filter, error) {

	var pbFilters []*pb.Filter

	for _, filter := range of.Filters {
		pbFilter, err := filter.toProto()
		if err != nil {
			return nil, err
		}
		pbFilters = append(pbFilters, pbFilter)
	}
	return &pb.Filter{FilterType: &pb.Filter_CompositeFilter{CompositeFilter: &pb.CompositeFilter{
		Op:      pb.CompositeFilter_OR,
		Filters: pbFilters,
	}}}, nil
}

func (of OrFilter) toValidFilter() (EntityFilter, error) {
	var validFilters []EntityFilter
	for _, filter := range of.Filters {
		validFilter, err := filter.toValidFilter()
		if err != nil {
			return nil, err
		}
		validFilters = append(validFilters, validFilter)
	}
	of.Filters = validFilters
	return of, nil
}

// AndFilter represents the intersection of two or more filters.
type AndFilter struct {
	Filters []EntityFilter
}

func (AndFilter) isCompositeFilter() {}

func (af AndFilter) toProto() (*pb.Filter, error) {

	var pbFilters []*pb.Filter

	for _, filter := range af.Filters {
		pbFilter, err := filter.toProto()
		if err != nil {
			return nil, err
		}
		pbFilters = append(pbFilters, pbFilter)
	}
	return &pb.Filter{FilterType: &pb.Filter_CompositeFilter{CompositeFilter: &pb.CompositeFilter{
		Op:      pb.CompositeFilter_AND,
		Filters: pbFilters,
	}}}, nil
}

func (af AndFilter) toValidFilter() (EntityFilter, error) {
	var validFilters []EntityFilter
	for _, filter := range af.Filters {
		validFilter, err := filter.toValidFilter()
		if err != nil {
			return nil, err
		}
		validFilters = append(validFilters, validFilter)
	}
	af.Filters = validFilters
	return af, nil
}

// NewQuery creates a new Query for a specific entity kind.
//
// An empty kind means to return all entities, including entities created and
// managed by other App Engine features, and is called a kindless query.
// Kindless queries cannot include filters or sort orders on property values.
func NewQuery(kind string) *Query {
	return &Query{
		kind:  kind,
		limit: -1,
	}
}

// Query represents a datastore query.
type Query struct {
	kind       string
	ancestor   *Key
	filter     []EntityFilter
	order      []order
	projection []string

	distinct   bool
	distinctOn []string
	keysOnly   bool
	eventual   bool
	limit      int32
	offset     int32
	start      []byte
	end        []byte

	namespace string

	trans *Transaction

	err error
}

func (q *Query) clone() *Query {
	x := *q
	// Copy the contents of the slice-typed fields to a new backing store.
	if len(q.filter) > 0 {
		x.filter = make([]EntityFilter, len(q.filter))
		copy(x.filter, q.filter)
	}
	if len(q.order) > 0 {
		x.order = make([]order, len(q.order))
		copy(x.order, q.order)
	}
	return &x
}

// Ancestor returns a derivative query with an ancestor filter.
// The ancestor should not be nil.
func (q *Query) Ancestor(ancestor *Key) *Query {
	q = q.clone()
	if ancestor == nil {
		q.err = errors.New("datastore: nil query ancestor")
		return q
	}
	q.ancestor = ancestor
	return q
}

// EventualConsistency returns a derivative query that returns eventually
// consistent results.
// It only has an effect on ancestor queries.
func (q *Query) EventualConsistency() *Query {
	q = q.clone()
	q.eventual = true
	return q
}

// Namespace returns a derivative query that is associated with the given
// namespace.
//
// A namespace may be used to partition data for multi-tenant applications.
// For details, see https://cloud.google.com/datastore/docs/concepts/multitenancy.
func (q *Query) Namespace(ns string) *Query {
	q = q.clone()
	q.namespace = ns
	return q
}

// Transaction returns a derivative query that is associated with the given
// transaction.
//
// All reads performed as part of the transaction will come from a single
// consistent snapshot. Furthermore, if the transaction is set to a
// serializable isolation level, another transaction cannot concurrently modify
// the data that is read or modified by this transaction.
func (q *Query) Transaction(t *Transaction) *Query {
	q = q.clone()
	q.trans = t
	return q
}

// FilterEntity returns a query with provided filter.
//
// Filter can be a single field comparison or a composite filter
// AndFilter and OrFilter are supported composite filters
// Filters in multiple calls are joined together by AND
func (q *Query) FilterEntity(ef EntityFilter) *Query {
	q = q.clone()
	vf, err := ef.toValidFilter()
	if err != nil {
		q.err = err
		return q
	}
	q.filter = append(q.filter, vf)
	return q
}

// Filter returns a derivative query with a field-based filter.
//
// Deprecated: Use the FilterField method instead, which supports the same
// set of operations (and more).
//
// The filterStr argument must be a field name followed by optional space,
// followed by an operator, one of ">", "<", ">=", "<=", "=", and "!=".
// Fields are compared against the provided value using the operator.
// Multiple filters are AND'ed together.
// Field names which contain spaces, quote marks, or operator characters
// should be passed as quoted Go string literals as returned by strconv.Quote
// or the fmt package's %q verb.
func (q *Query) Filter(filterStr string, value interface{}) *Query {
	// TODO( #5977 ): Add better string parsing (or something)
	filterStr = strings.TrimSpace(filterStr)
	if filterStr == "" {
		q.err = fmt.Errorf("datastore: invalid filter %q", filterStr)
		return q
	}
	f := strings.TrimRight(filterStr, " ><=!")
	op := strings.TrimSpace(filterStr[len(f):])
	return q.FilterField(f, op, value)
}

// FilterField returns a derivative query with a field-based filter.
// The operation parameter takes the following strings: ">", "<", ">=", "<=",
// "=", "!=", "in", and "not-in".
// Fields are compared against the provided value using the operator.
// Multiple filters are AND'ed together.
// Field names which contain spaces, quote marks, or operator characters
// should be passed as quoted Go string literals as returned by strconv.Quote
// or the fmt package's %q verb.
// For "in" and "not-in" operator, use []interface{} as value. For instance
// query.FilterField("Month", "in", []interface{}{1, 2, 3, 4})
func (q *Query) FilterField(fieldName, operator string, value interface{}) *Query {
	return q.FilterEntity(PropertyFilter{
		FieldName: fieldName,
		Operator:  operator,
		Value:     value,
	})
}

// Order returns a derivative query with a field-based sort order. Orders are
// applied in the order they are added. The default order is ascending; to sort
// in descending order prefix the fieldName with a minus sign (-).
// Field names which contain spaces, quote marks, or the minus sign
// should be passed as quoted Go string literals as returned by strconv.Quote
// or the fmt package's %q verb.
func (q *Query) Order(fieldName string) *Query {
	q = q.clone()
	fieldName, dir := strings.TrimSpace(fieldName), ascending
	if strings.HasPrefix(fieldName, "-") {
		fieldName, dir = strings.TrimSpace(fieldName[1:]), descending
	} else if strings.HasPrefix(fieldName, "+") {
		q.err = fmt.Errorf("datastore: invalid order: %q", fieldName)
		return q
	}
	fieldName, err := unquote(fieldName)
	if err != nil {
		q.err = fmt.Errorf("datastore: invalid syntax for quoted field name %q", fieldName)
		return q
	}
	if fieldName == "" {
		q.err = errors.New("datastore: empty order")
		return q
	}
	q.order = append(q.order, order{
		Direction: dir,
		FieldName: fieldName,
	})
	return q
}

// unquote optionally interprets s as a double-quoted or backquoted Go
// string literal if it begins with the relevant character.
func unquote(s string) (string, error) {
	if s == "" || (s[0] != '`' && s[0] != '"') {
		return s, nil
	}
	return strconv.Unquote(s)
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
// queries. Distinct cannot be used with DistinctOn.
func (q *Query) Distinct() *Query {
	q = q.clone()
	q.distinct = true
	return q
}

// DistinctOn returns a derivative query that yields de-duplicated entities with
// respect to the set of the specified fields. It is only used for projection
// queries. The field list should be a subset of the projected field list.
// DistinctOn cannot be used with Distinct.
func (q *Query) DistinctOn(fieldNames ...string) *Query {
	q = q.clone()
	q.distinctOn = fieldNames
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

// Start returns a derivative query with the given start point.
func (q *Query) Start(c Cursor) *Query {
	q = q.clone()
	q.start = c.cc
	return q
}

// End returns a derivative query with the given end point.
func (q *Query) End(c Cursor) *Query {
	q = q.clone()
	q.end = c.cc
	return q
}

// toRunQueryRequest converts the query to a protocol buffer.
func (q *Query) toRunQueryRequest(req *pb.RunQueryRequest) error {
	dst, err := q.toProto()
	if err != nil {
		return err
	}

	req.ReadOptions, err = parseQueryReadOptions(q.eventual, q.trans)
	if err != nil {
		return err
	}

	req.QueryType = &pb.RunQueryRequest_Query{Query: dst}
	return nil
}

func (q *Query) toProto() (*pb.Query, error) {
	if len(q.projection) != 0 && q.keysOnly {
		return nil, errors.New("datastore: query cannot both project and be keys-only")
	}
	if len(q.distinctOn) != 0 && q.distinct {
		return nil, errors.New("datastore: query cannot be both distinct and distinct-on")
	}
	dst := &pb.Query{}
	if q.kind != "" {
		dst.Kind = []*pb.KindExpression{{Name: q.kind}}
	}
	if q.projection != nil {
		for _, propertyName := range q.projection {
			dst.Projection = append(dst.Projection, &pb.Projection{Property: &pb.PropertyReference{Name: propertyName}})
		}

		for _, propertyName := range q.distinctOn {
			dst.DistinctOn = append(dst.DistinctOn, &pb.PropertyReference{Name: propertyName})
		}

		if q.distinct {
			for _, propertyName := range q.projection {
				dst.DistinctOn = append(dst.DistinctOn, &pb.PropertyReference{Name: propertyName})
			}
		}
	}
	if q.keysOnly {
		dst.Projection = []*pb.Projection{{Property: &pb.PropertyReference{Name: keyFieldName}}}
	}
	var filters []*pb.Filter
	for _, qf := range q.filter {
		pbFilter, err := qf.toProto()
		if err != nil {
			return nil, err
		}
		filters = append(filters, pbFilter)
	}

	if q.ancestor != nil {
		filters = append(filters, &pb.Filter{
			FilterType: &pb.Filter_PropertyFilter{PropertyFilter: &pb.PropertyFilter{
				Property: &pb.PropertyReference{Name: keyFieldName},
				Op:       pb.PropertyFilter_HAS_ANCESTOR,
				Value:    &pb.Value{ValueType: &pb.Value_KeyValue{KeyValue: keyToProto(q.ancestor)}},
			}}})
	}

	if len(filters) == 1 {
		dst.Filter = filters[0]
	} else if len(filters) > 1 {
		dst.Filter = &pb.Filter{FilterType: &pb.Filter_CompositeFilter{CompositeFilter: &pb.CompositeFilter{
			Op:      pb.CompositeFilter_AND,
			Filters: filters,
		}}}
	}

	for _, qo := range q.order {
		if qo.FieldName == "" {
			return nil, errors.New("datastore: empty query order field name")
		}
		xo := &pb.PropertyOrder{
			Property:  &pb.PropertyReference{Name: qo.FieldName},
			Direction: sortDirectionToProto[qo.Direction],
		}
		dst.Order = append(dst.Order, xo)
	}
	if q.limit >= 0 {
		dst.Limit = &wrapperspb.Int32Value{Value: q.limit}
	}
	dst.Offset = q.offset
	dst.StartCursor = q.start
	dst.EndCursor = q.end

	return dst, nil
}

// Count returns the number of results for the given query.
//
// The running time and number of API calls made by Count scale linearly with
// the sum of the query's offset and limit. Unless the result count is
// expected to be small, it is best to specify a limit; otherwise Count will
// continue until it finishes counting or the provided context expires.
//
// Deprecated. Use Client.RunAggregationQuery() instead.
func (c *Client) Count(ctx context.Context, q *Query) (n int, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.Count")
	defer func() { trace.EndSpan(ctx, err) }()

	// Check that the query is well-formed.
	if q.err != nil {
		return 0, q.err
	}

	// Create a copy of the query, with keysOnly true (if we're not a projection,
	// since the two are incompatible).
	newQ := q.clone()
	newQ.keysOnly = len(newQ.projection) == 0

	// Create an iterator and use it to walk through the batches of results
	// directly.
	it := c.Run(ctx, newQ)
	for {
		err := it.nextBatch()
		if err == iterator.Done {
			return n, nil
		}
		if err != nil {
			return 0, err
		}
		n += len(it.results)
	}
}

// RunOption lets the user provide options while running a query
type RunOption interface {
	apply(*runQuerySettings) error
}

// ExplainOptions is explain options for the query.
//
// Query Explain feature is still in preview and not yet publicly available.
// Pre-GA features might have limited support and can change at any time.
type ExplainOptions struct {
	// When false (the default), the query will be planned, returning only
	// metrics from the planning stages.
	// When true, the query will be planned and executed, returning the full
	// query results along with both planning and execution stage metrics.
	Analyze bool
}

func (e ExplainOptions) apply(s *runQuerySettings) error {
	if s.explainOptions != nil {
		return errors.New("datastore: ExplainOptions can be specified only once")
	}
	pbExplainOptions := pb.ExplainOptions{
		Analyze: e.Analyze,
	}
	s.explainOptions = &pbExplainOptions
	return nil
}

type runQuerySettings struct {
	explainOptions *pb.ExplainOptions
}

// newRunQuerySettings creates a runQuerySettings with a given RunOption slice.
func newRunQuerySettings(opts []RunOption) (*runQuerySettings, error) {
	s := &runQuerySettings{}
	for _, o := range opts {
		if o == nil {
			return nil, errors.New("datastore: RunOption cannot be nil")
		}
		err := o.apply(s)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ExplainMetrics for the query.
type ExplainMetrics struct {
	// Planning phase information for the query.
	PlanSummary *PlanSummary

	// Aggregated stats from the execution of the query. Only present when
	// ExplainOptions.Analyze is set to true.
	ExecutionStats *ExecutionStats
}

// PlanSummary represents planning phase information for the query.
type PlanSummary struct {
	// The indexes selected for the query. For example:
	//
	//	[
	//	  {"query_scope": "Collection", "properties": "(foo ASC, __name__ ASC)"},
	//	  {"query_scope": "Collection", "properties": "(bar ASC, __name__ ASC)"}
	//	]
	IndexesUsed []*map[string]interface{}
}

// ExecutionStats represents execution statistics for the query.
type ExecutionStats struct {
	// Total number of results returned, including documents, projections,
	// aggregation results, keys.
	ResultsReturned int64
	// Total time to execute the query in the backend.
	ExecutionDuration *time.Duration
	// Total billable read operations.
	ReadOperations int64
	// Debugging statistics from the execution of the query. Note that the
	// debugging stats are subject to change as Firestore evolves. It could
	// include:
	//
	//	{
	//	  "indexes_entries_scanned": "1000",
	//	  "documents_scanned": "20",
	//	  "billing_details" : {
	//	     "documents_billable": "20",
	//	     "index_entries_billable": "1000",
	//	     "min_query_cost": "0"
	//	  }
	//	}
	DebugStats *map[string]interface{}
}

// GetAllWithOptionsResult is the result of call to GetAllWithOptions method
type GetAllWithOptionsResult struct {
	Keys []*Key

	// Query explain metrics. This is only present when ExplainOptions is provided.
	ExplainMetrics *ExplainMetrics
}

// GetAll runs the provided query in the given context and returns all keys
// that match that query, as well as appending the values to dst.
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
// If q is a “keys-only” query, GetAll ignores dst and only returns the keys.
//
// The running time and number of API calls made by GetAll scale linearly with
// with the sum of the query's offset and limit. Unless the result count is
// expected to be small, it is best to specify a limit; otherwise GetAll will
// continue until it finishes collecting results or the provided context
// expires.
func (c *Client) GetAll(ctx context.Context, q *Query, dst interface{}) (keys []*Key, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.GetAll")
	defer func() { trace.EndSpan(ctx, err) }()

	res, err := c.GetAllWithOptions(ctx, q, dst)
	return res.Keys, err
}

// GetAllWithOptions is similar to GetAll but runs the query with provided options
func (c *Client) GetAllWithOptions(ctx context.Context, q *Query, dst interface{}, opts ...RunOption) (res GetAllWithOptionsResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.GetAllWithOptions")
	defer func() { trace.EndSpan(ctx, err) }()

	var (
		dv               reflect.Value
		mat              multiArgType
		elemType         reflect.Type
		errFieldMismatch error
	)
	if !q.keysOnly {
		dv = reflect.ValueOf(dst)
		if dv.Kind() != reflect.Ptr || dv.IsNil() {
			return res, ErrInvalidEntityType
		}
		dv = dv.Elem()
		mat, elemType = checkMultiArg(dv)
		if mat == multiArgTypeInvalid || mat == multiArgTypeInterface {
			return res, ErrInvalidEntityType
		}
	}

	for t := c.RunWithOptions(ctx, q, opts...); ; {
		k, e, err := t.next()
		res.ExplainMetrics = t.ExplainMetrics
		if err == iterator.Done {
			break
		}
		if err != nil {
			return res, err
		}
		if !q.keysOnly {
			ev := reflect.New(elemType)
			if elemType.Kind() == reflect.Map {
				// This is a special case. The zero values of a map type are
				// not immediately useful; they have to be make'd.
				//
				// Funcs and channels are similar, in that a zero value is not useful,
				// but even a freshly make'd channel isn't useful: there's no fixed
				// channel buffer size that is always going to be large enough, and
				// there's no goroutine to drain the other end. Theoretically, these
				// types could be supported, for example by sniffing for a constructor
				// method or requiring prior registration, but for now it's not a
				// frequent enough concern to be worth it. Programmers can work around
				// it by explicitly using Iterator.Next instead of the Query.GetAll
				// convenience method.
				x := reflect.MakeMap(elemType)
				ev.Elem().Set(x)
			}
			if err = loadEntityProto(ev.Interface(), e); err != nil {
				if _, ok := err.(*ErrFieldMismatch); ok {
					// We continue loading entities even in the face of field mismatch errors.
					// If we encounter any other error, that other error is returned. Otherwise,
					// an ErrFieldMismatch is returned.
					errFieldMismatch = err
				} else {
					return res, err
				}
			}
			if mat != multiArgTypeStructPtr {
				ev = ev.Elem()
			}
			dv.Set(reflect.Append(dv, ev))
		}
		res.Keys = append(res.Keys, k)
	}
	return res, errFieldMismatch
}

// Run runs the given query in the given context
func (c *Client) Run(ctx context.Context, q *Query) (it *Iterator) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.Run")
	defer func() { trace.EndSpan(ctx, it.err) }()
	return c.run(ctx, q)
}

// RunWithOptions runs the given query in the given context with the provided options
func (c *Client) RunWithOptions(ctx context.Context, q *Query, opts ...RunOption) (it *Iterator) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.RunWithOptions")
	defer func() { trace.EndSpan(ctx, it.err) }()
	return c.run(ctx, q, opts...)
}

// run runs the given query in the given context with the provided options
func (c *Client) run(ctx context.Context, q *Query, opts ...RunOption) *Iterator {
	if q.err != nil {
		return &Iterator{ctx: ctx, err: q.err}
	}
	t := &Iterator{
		ctx:          ctx,
		client:       c,
		limit:        q.limit,
		offset:       q.offset,
		keysOnly:     q.keysOnly,
		pageCursor:   q.start,
		entityCursor: q.start,
		req: &pb.RunQueryRequest{
			ProjectId:  c.dataset,
			DatabaseId: c.databaseID,
		},
		trans:    q.trans,
		eventual: q.eventual,
	}

	if q.namespace != "" {
		t.req.PartitionId = &pb.PartitionId{
			NamespaceId: q.namespace,
		}
	}

	runSettings, err := newRunQuerySettings(opts)
	if err != nil {
		t.err = err
		return t
	}

	if runSettings.explainOptions != nil {
		t.req.ExplainOptions = runSettings.explainOptions
	}

	if err := q.toRunQueryRequest(t.req); err != nil {
		t.err = err
	}
	return t
}

// RunAggregationQuery gets aggregation query (e.g. COUNT) results from the service.
func (c *Client) RunAggregationQuery(ctx context.Context, aq *AggregationQuery) (ar AggregationResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.RunAggregationQuery")
	defer func() { trace.EndSpan(ctx, err) }()
	aro, err := c.RunAggregationQueryWithOptions(ctx, aq)
	return aro.Result, err
}

// RunAggregationQueryWithOptions runs aggregation query (e.g. COUNT) with provided options and returns results from the service.
func (c *Client) RunAggregationQueryWithOptions(ctx context.Context, aq *AggregationQuery, opts ...RunOption) (ar AggregationWithOptionsResult, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.Query.RunAggregationQueryWithOptions")
	defer func() { trace.EndSpan(ctx, err) }()

	if aq == nil {
		return ar, errors.New("datastore: aggregation query cannot be nil")
	}

	if aq.query == nil {
		return ar, errors.New("datastore: aggregation query must include nested query")
	}

	if len(aq.aggregationQueries) == 0 {
		return ar, errors.New("datastore: aggregation query must contain one or more operators (e.g. count)")
	}

	q, err := aq.query.toProto()
	if err != nil {
		return ar, err
	}

	req := &pb.RunAggregationQueryRequest{
		ProjectId:  c.dataset,
		DatabaseId: c.databaseID,
		QueryType: &pb.RunAggregationQueryRequest_AggregationQuery{
			AggregationQuery: &pb.AggregationQuery{
				QueryType: &pb.AggregationQuery_NestedQuery{
					NestedQuery: q,
				},
				Aggregations: aq.aggregationQueries,
			},
		},
	}

	if aq.query.namespace != "" {
		req.PartitionId = &pb.PartitionId{
			NamespaceId: aq.query.namespace,
		}
	}

	runSettings, err := newRunQuerySettings(opts)
	if err != nil {
		return ar, err
	}
	if runSettings.explainOptions != nil {
		req.ExplainOptions = runSettings.explainOptions
	}

	// Parse the read options.
	txn := aq.query.trans
	if txn != nil {
		defer txn.stateLockDeferUnlock()()
	}

	req.ReadOptions, err = parseQueryReadOptions(aq.query.eventual, txn)
	if err != nil {
		return ar, err
	}

	resp, err := c.client.RunAggregationQuery(ctx, req)
	if err != nil {
		return ar, err
	}

	if txn != nil && txn.state == transactionStateNotStarted {
		txn.setToInProgress(resp.Transaction)
	}

	if req.ExplainOptions == nil || req.ExplainOptions.Analyze {
		ar.Result = make(AggregationResult)
		// TODO(developer): change batch parsing logic if other aggregations are supported.
		for _, a := range resp.Batch.AggregationResults {
			for k, v := range a.AggregateProperties {
				ar.Result[k] = v
			}
		}
	}

	ar.ExplainMetrics = fromPbExplainMetrics(resp.GetExplainMetrics())
	return ar, nil
}

func validateReadOptions(eventual bool, t *Transaction) error {
	if t == nil {
		return nil
	}
	if eventual {
		return errEventualConsistencyTransaction
	}
	if t.state == transactionStateExpired {
		return errExpiredTransaction
	}
	return nil
}

// parseQueryReadOptions translates Query read options into protobuf format.
func parseQueryReadOptions(eventual bool, t *Transaction) (*pb.ReadOptions, error) {
	err := validateReadOptions(eventual, t)
	if err != nil {
		return nil, err
	}

	if t != nil {
		return t.parseReadOptions()
	}

	if eventual {
		return &pb.ReadOptions{ConsistencyType: &pb.ReadOptions_ReadConsistency_{ReadConsistency: pb.ReadOptions_EVENTUAL}}, nil
	}

	return nil, nil
}

// Iterator is the result of running a query.
//
// It is not safe for concurrent use.
type Iterator struct {
	ctx    context.Context
	client *Client
	err    error

	// results is the list of EntityResults still to be iterated over from the
	// most recent API call. It will be nil if no requests have yet been issued.
	results []*pb.EntityResult
	// req is the request to send. It may be modified and used multiple times.
	req *pb.RunQueryRequest

	// limit is the limit on the number of results this iterator should return.
	// The zero value is used to prevent further fetches from the server.
	// A negative value means unlimited.
	limit int32
	// offset is the number of results that still need to be skipped.
	offset int32
	// keysOnly records whether the query was keys-only (skip entity loading).
	keysOnly bool

	// pageCursor is the compiled cursor for the next batch/page of result.
	// TODO(djd): Can we delete this in favour of paging with the last
	// entityCursor from each batch?
	pageCursor []byte
	// entityCursor is the compiled cursor of the next result.
	entityCursor []byte

	// Query explain metrics. This is only present when ExplainOptions is used.
	ExplainMetrics *ExplainMetrics

	// trans records the transaction in which the query was run
	trans *Transaction

	// eventual records whether the query was eventual
	eventual bool
}

// Next returns the key of the next result. When there are no more results,
// iterator.Done is returned as the error.
//
// If the query is not keys only and dst is non-nil, it also loads the entity
// stored for that key into the struct pointer or PropertyLoadSaver dst, with
// the same semantics and possible errors as for the Get function.
func (t *Iterator) Next(dst interface{}) (k *Key, err error) {
	k, e, err := t.next()
	if err != nil {
		return nil, err
	}
	if dst != nil && !t.keysOnly {
		err = loadEntityProto(dst, e)
	}
	return k, err
}

func (t *Iterator) next() (*Key, *pb.Entity, error) {
	// Fetch additional batches while there are no more results.
	for t.err == nil && len(t.results) == 0 {
		t.err = t.nextBatch()
	}
	if t.err != nil {
		return nil, nil, t.err
	}

	// Extract the next result, update cursors, and parse the entity's key.
	e := t.results[0]
	t.results = t.results[1:]
	t.entityCursor = e.Cursor
	if len(t.results) == 0 {
		t.entityCursor = t.pageCursor // At the end of the batch.
	}
	if e.Entity.Key == nil {
		return nil, nil, errors.New("datastore: internal error: server did not return a key")
	}
	k, err := protoToKey(e.Entity.Key)
	if err != nil || k.Incomplete() {
		return nil, nil, errors.New("datastore: internal error: server returned an invalid key")
	}

	return k, e.Entity, nil
}

// nextBatch makes a single call to the server for a batch of results.
func (t *Iterator) nextBatch() error {
	if t.err != nil {
		return t.err
	}

	if t.limit == 0 {
		return iterator.Done // Short-circuits the zero-item response.
	}

	// Adjust the query with the latest start cursor, limit and offset.
	q := t.req.GetQuery()
	q.StartCursor = t.pageCursor
	q.Offset = t.offset
	if t.limit >= 0 {
		q.Limit = &wrapperspb.Int32Value{Value: t.limit}
	} else {
		q.Limit = nil
	}

	txn := t.trans
	if txn != nil {
		defer txn.stateLockDeferUnlock()()
	}

	var err error
	t.req.ReadOptions, err = parseQueryReadOptions(t.eventual, txn)
	if err != nil {
		return err
	}

	// Run the query.
	resp, err := t.client.client.RunQuery(t.ctx, t.req)
	if err != nil {
		return err
	}

	if txn != nil && txn.state == transactionStateNotStarted {
		txn.setToInProgress(resp.Transaction)
	}

	if t.req.ExplainOptions != nil && !t.req.ExplainOptions.Analyze {
		// No results to process
		t.limit = 0
		t.ExplainMetrics = fromPbExplainMetrics(resp.GetExplainMetrics())
		return nil
	}

	// Adjust any offset from skipped results.
	skip := resp.Batch.SkippedResults
	if skip < 0 {
		return errors.New("datastore: internal error: negative number of skipped_results")
	}
	t.offset -= skip
	if t.offset < 0 {
		return errors.New("datastore: internal error: query skipped too many results")
	}
	if t.offset > 0 && len(resp.Batch.EntityResults) > 0 {
		return errors.New("datastore: internal error: query returned results before requested offset")
	}

	// Adjust the limit.
	if t.limit >= 0 {
		t.limit -= int32(len(resp.Batch.EntityResults))
		if t.limit < 0 {
			return errors.New("datastore: internal error: query returned more results than the limit")
		}
	}

	// If there are no more results available, set limit to zero to prevent
	// further fetches. Otherwise, check that there is a next page cursor available.
	if resp.Batch.MoreResults != pb.QueryResultBatch_NOT_FINISHED {
		t.limit = 0
	} else if resp.Batch.EndCursor == nil {
		return errors.New("datastore: internal error: server did not return a cursor")
	}

	// Update cursors.
	// If any results were skipped, use the SkippedCursor as the next entity cursor.
	if skip > 0 {
		t.entityCursor = resp.Batch.SkippedCursor
	} else {
		t.entityCursor = q.StartCursor
	}
	t.pageCursor = resp.Batch.EndCursor

	t.results = resp.Batch.EntityResults
	t.ExplainMetrics = fromPbExplainMetrics(resp.GetExplainMetrics())
	return nil
}

func fromPbExplainMetrics(pbExplainMetrics *pb.ExplainMetrics) *ExplainMetrics {
	if pbExplainMetrics == nil {
		return nil
	}
	explainMetrics := &ExplainMetrics{
		PlanSummary:    fromPbPlanSummary(pbExplainMetrics.PlanSummary),
		ExecutionStats: fromPbExecutionStats(pbExplainMetrics.ExecutionStats),
	}
	return explainMetrics
}

func fromPbPlanSummary(pbPlanSummary *pb.PlanSummary) *PlanSummary {
	if pbPlanSummary == nil {
		return nil
	}

	planSummary := &PlanSummary{}
	indexesUsed := []*map[string]interface{}{}
	for _, pbIndexUsed := range pbPlanSummary.GetIndexesUsed() {
		indexUsed := protostruct.DecodeToMap(pbIndexUsed)
		indexesUsed = append(indexesUsed, &indexUsed)
	}

	planSummary.IndexesUsed = indexesUsed
	return planSummary
}

func fromPbExecutionStats(pbstats *pb.ExecutionStats) *ExecutionStats {
	if pbstats == nil {
		return nil
	}

	executionStats := &ExecutionStats{
		ResultsReturned: pbstats.GetResultsReturned(),
		ReadOperations:  pbstats.GetReadOperations(),
	}

	executionDuration := pbstats.GetExecutionDuration().AsDuration()
	executionStats.ExecutionDuration = &executionDuration

	debugStats := protostruct.DecodeToMap(pbstats.GetDebugStats())
	executionStats.DebugStats = &debugStats

	return executionStats
}

// Cursor returns a cursor for the iterator's current location.
func (t *Iterator) Cursor() (c Cursor, err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Query.Cursor")
	defer func() { trace.EndSpan(t.ctx, err) }()

	// If there is still an offset, we need to the skip those results first.
	for t.err == nil && t.offset > 0 {
		t.err = t.nextBatch()
	}

	if t.err != nil && t.err != iterator.Done {
		return Cursor{}, t.err
	}

	return Cursor{t.entityCursor}, nil
}

// Cursor is an iterator's position. It can be converted to and from an opaque
// string. A cursor can be used from different HTTP requests, but only with a
// query with the same kind, ancestor, filter and order constraints.
//
// The zero Cursor can be used to indicate that there is no start and/or end
// constraint for a query.
type Cursor struct {
	cc []byte
}

// String returns a base-64 string representation of a cursor.
func (c Cursor) String() string {
	if c.cc == nil {
		return ""
	}

	return strings.TrimRight(base64.URLEncoding.EncodeToString(c.cc), "=")
}

// DecodeCursor decodes a cursor from its base-64 string representation.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	if n := len(s) % 4; n != 0 {
		s += strings.Repeat("=", 4-n)
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, err
	}
	return Cursor{b}, nil
}

// NewAggregationQuery returns an AggregationQuery with this query as its
// base query.
func (q *Query) NewAggregationQuery() *AggregationQuery {
	return &AggregationQuery{
		query:              q,
		aggregationQueries: make([]*pb.AggregationQuery_Aggregation, 0),
	}
}

// AggregationQuery allows for generating aggregation results of an underlying
// basic query. A single AggregationQuery can contain multiple aggregations.
type AggregationQuery struct {
	query              *Query                             // query contains a reference pointer to the underlying structured query.
	aggregationQueries []*pb.AggregationQuery_Aggregation // aggregateQueries contains all of the queries for this request.
}

// WithCount specifies that the aggregation query provide a count of results
// returned by the underlying Query.
func (aq *AggregationQuery) WithCount(alias string) *AggregationQuery {
	if alias == "" {
		alias = fmt.Sprintf("%s_%s", "count", aq.query.kind)
	}

	aqpb := &pb.AggregationQuery_Aggregation{
		Alias:    alias,
		Operator: &pb.AggregationQuery_Aggregation_Count_{},
	}

	aq.aggregationQueries = append(aq.aggregationQueries, aqpb)

	return aq
}

// WithSum specifies that the aggregation query should provide a sum of the values
// of the provided field in the results returned by the underlying Query.
// The alias argument can be empty or a valid Datastore entity property name. It can be used
// as key in the AggregationResult to get the sum value. If alias is empty, Datastore
// will autogenerate a key.
func (aq *AggregationQuery) WithSum(fieldName string, alias string) *AggregationQuery {
	aqpb := &pb.AggregationQuery_Aggregation{
		Alias: alias,
		Operator: &pb.AggregationQuery_Aggregation_Sum_{
			Sum: &pb.AggregationQuery_Aggregation_Sum{
				Property: &pb.PropertyReference{
					Name: fieldName,
				},
			},
		},
	}

	aq.aggregationQueries = append(aq.aggregationQueries, aqpb)

	return aq
}

// WithAvg specifies that the aggregation query should provide an average of the values
// of the provided field in the results returned by the underlying Query.
// The alias argument can be empty or a valid Datastore entity property name. It can be used
// as key in the AggregationResult to get the sum value. If alias is empty, Datastore
// will autogenerate a key.
func (aq *AggregationQuery) WithAvg(fieldName string, alias string) *AggregationQuery {
	aqpb := &pb.AggregationQuery_Aggregation{
		Alias: alias,
		Operator: &pb.AggregationQuery_Aggregation_Avg_{
			Avg: &pb.AggregationQuery_Aggregation_Avg{
				Property: &pb.PropertyReference{
					Name: fieldName,
				},
			},
		},
	}

	aq.aggregationQueries = append(aq.aggregationQueries, aqpb)

	return aq
}

// AggregationResult contains the results of an aggregation query.
type AggregationResult map[string]interface{}

// AggregationWithOptionsResult contains the results of an aggregation query run with options.
type AggregationWithOptionsResult struct {
	Result AggregationResult

	// Query explain metrics. This is only present when ExplainOptions is provided.
	ExplainMetrics *ExplainMetrics
}
