package spanner

import (
	"strings"

	"golang.org/x/net/context"

	"encoding/base64"
	"reflect"
	"strconv"
	"time"

	"cloud.google.com/go/civil"
	proto3 "github.com/golang/protobuf/ptypes/struct"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc/codes"
)

const (
	// minBackoff is the minimum backoff used by default.
	minBackoff = 50 * time.Millisecond
	// maxBackoff is the maximum backoff used by default.
	maxBackoff = 10 * time.Second
	// jitter is the jitter factor.
	jitter = 0.4
	// rate is the rate of exponential increase in the backoff.
	rate = 1.3
)

// ReadUsingIndexLimit returns a RowIterator for reading multiple rows from the database
// using an index. Returns a limited number of results, or infinite if limit is zero.
//
// Currently, this function can only read columns that are part of the index
// key, part of the primary key, or stored in the index due to a STORING clause
// in the index definition.
func (t *txReadOnly) ReadUsingIndexLimit(ctx context.Context, table, index string, keys KeySet, columns []string, limit int64) *RowIterator {
	var (
		sh  *sessionHandle
		ts  *sppb.TransactionSelector
		err error
	)
	kset, err := keys.keySetProto()
	if err != nil {
		return &RowIterator{err: err}
	}
	if sh, ts, err = t.acquire(ctx); err != nil {
		return &RowIterator{err: err}
	}
	// Cloud Spanner will return "Session not found" on bad sessions.
	sid, client := sh.getID(), sh.getClient()
	if sid == "" || client == nil {
		// Might happen if transaction is closed in the middle of a API call.
		return &RowIterator{err: errSessionClosed(sh)}
	}
	return stream(
		contextWithOutgoingMetadata(ctx, sh.getMetadata()),
		func(ctx context.Context, resumeToken []byte) (streamingReceiver, error) {
			return client.StreamingRead(ctx,
				&sppb.ReadRequest{
					Session:     sid,
					Transaction: ts,
					Table:       table,
					Index:       index,
					Columns:     columns,
					KeySet:      kset,
					ResumeToken: resumeToken,
					Limit:       limit,
				})
		},
		t.setTimestamp,
		t.release,
	)
}

func DistinctKeys(keys ...Key) KeySet {
	return distinct(keys)
}

type distinct []Key

func (d distinct) keySetProto() (*sppb.KeySet, error) {
	upb := &sppb.KeySet{Keys: make([]*proto3.ListValue, len(d))}
	for j, k := range d {
		pb, err := k.proto()
		if err != nil {
			return nil, err
		}
		upb.Keys[j] = pb
	}
	return upb, nil
}

// decodeValue decodes a protobuf Value into a pointer to a Go value, as
// specified by sppb.Type.
func decodeValue(v *proto3.Value, t *sppb.Type, ptr interface{}) error {
	if v == nil {
		return errNilSrc()
	}
	if t == nil {
		return errNilSpannerType()
	}
	code := t.Code
	acode := sppb.TypeCode_TYPE_CODE_UNSPECIFIED
	if code == sppb.TypeCode_ARRAY {
		if t.ArrayElementType == nil {
			return errNilArrElemType(t)
		}
		acode = t.ArrayElementType.Code
	}
	_, isNull := v.Kind.(*proto3.Value_NullValue)

	// Do the decoding based on the type of ptr.
	switch p := ptr.(type) {
	case nil:
		return errNilDst(nil)
	case *string:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_STRING {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = ""
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		*p = x
	case *NullString:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_STRING {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = NullString{}
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		p.Valid = true
		p.StringVal = x
	case *[]NullString:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_STRING {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeNullStringArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]string:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_STRING {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeStringArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]byte:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_BYTES {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		y, err := base64.StdEncoding.DecodeString(x)
		if err != nil {
			return errBadEncoding(v, err)
		}
		*p = y
	case *[][]byte:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_BYTES {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeByteArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *int64:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_INT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = 0
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		y, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return errBadEncoding(v, err)
		}
		*p = y
	case *NullInt64:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_INT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = NullInt64{}
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		y, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return errBadEncoding(v, err)
		}
		p.Valid = true
		p.Int64 = y
	case *[]NullInt64:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_INT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeNullInt64Array(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]int64:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_INT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeInt64Array(x)
		if err != nil {
			return err
		}
		*p = y
	case *bool:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_BOOL {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = false
			break
		}
		x, err := getBoolValue(v)
		if err != nil {
			return err
		}
		*p = x
	case *NullBool:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_BOOL {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = NullBool{}
			break
		}
		x, err := getBoolValue(v)
		if err != nil {
			return err
		}
		p.Valid = true
		p.Bool = x
	case *[]NullBool:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_BOOL {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeNullBoolArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]bool:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_BOOL {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeBoolArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *float64:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_FLOAT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = 0
			break
		}
		x, err := getFloat64Value(v)
		if err != nil {
			return err
		}
		*p = x
	case *NullFloat64:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_FLOAT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = NullFloat64{}
			break
		}
		x, err := getFloat64Value(v)
		if err != nil {
			return err
		}
		p.Valid = true
		p.Float64 = x
	case *[]NullFloat64:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_FLOAT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeNullFloat64Array(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]float64:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_FLOAT64 {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeFloat64Array(x)
		if err != nil {
			return err
		}
		*p = y
	case *time.Time:
		var nt NullTime
		if isNull {
			*p = time.Time{}
			break
		}
		err := parseNullTime(v, &nt, code, isNull)
		if err != nil {
			return nil
		}
		*p = nt.Time
	case *NullTime:
		err := parseNullTime(v, p, code, isNull)
		if err != nil {
			return err
		}
	case *[]NullTime:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_TIMESTAMP {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeNullTimeArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]time.Time:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_TIMESTAMP {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeTimeArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *civil.Date:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_DATE {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = civil.Date{}
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		y, err := civil.ParseDate(x)
		if err != nil {
			return errBadEncoding(v, err)
		}
		*p = y
	case *NullDate:
		if p == nil {
			return errNilDst(p)
		}
		if code != sppb.TypeCode_DATE {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = NullDate{}
			break
		}
		x, err := getStringValue(v)
		if err != nil {
			return err
		}
		y, err := civil.ParseDate(x)
		if err != nil {
			return errBadEncoding(v, err)
		}
		p.Valid = true
		p.Date = y
	case *[]NullDate:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_DATE {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeNullDateArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]civil.Date:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_DATE {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeDateArray(x)
		if err != nil {
			return err
		}
		*p = y
	case *[]NullRow:
		if p == nil {
			return errNilDst(p)
		}
		if acode != sppb.TypeCode_STRUCT {
			return errTypeMismatch(code, acode, ptr)
		}
		if isNull {
			*p = nil
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		y, err := decodeRowArray(t.ArrayElementType.StructType, x)
		if err != nil {
			return err
		}
		*p = y
	case *GenericColumnValue:
		*p = GenericColumnValue{Type: t, Value: v}
	default:
		// Check if the proto encoding is for an array of structs.
		if !(code == sppb.TypeCode_ARRAY && acode == sppb.TypeCode_STRUCT) {
			return errTypeMismatch(code, acode, ptr)
		}
		vp := reflect.ValueOf(p)
		if !vp.IsValid() {
			return errNilDst(p)
		}
		if !isPtrStructPtrSlice(vp.Type()) {
			// The container is not a pointer to a struct pointer slice.
			return errTypeMismatch(code, acode, ptr)
		}
		// Only use reflection for nil detection on slow path.
		// Also, IsNil panics on many types, so check it after the type check.
		if vp.IsNil() {
			return errNilDst(p)
		}
		if isNull {
			// The proto Value is encoding NULL, set the pointer to struct
			// slice to nil as well.
			vp.Elem().Set(reflect.Zero(vp.Elem().Type()))
			break
		}
		x, err := getListValue(v)
		if err != nil {
			return err
		}
		if err = decodeStructArray(t.ArrayElementType.StructType, x, p); err != nil {
			return err
		}
	}
	return nil
}

func IsRetryable(err error) bool {
	return isRetryable(err) || isAbortErr(err)
}

// isErrorClosing reports whether the error is generated by gRPC layer talking to a closed server.
func isErrorClosing(err error) bool {
	if err == nil {
		return false
	}
	if ErrCode(err) == codes.Internal && strings.Contains(ErrDesc(err), "transport is closing") {
		// Handle the case when connection is closed unexpectedly.
		// TODO: once gRPC is able to categorize
		// this as retryable error, we should stop parsing the
		// error message here.
		return true
	}
	return false
}

// isErrorRST reports whether the error is generated by gRPC client receiving a RST frame from server.
func isErrorRST(err error) bool {
	if err == nil {
		return false
	}
	if ErrCode(err) == codes.Internal && strings.Contains(ErrDesc(err), "stream terminated by RST_STREAM") {
		// TODO: once gRPC is able to categorize this error as "go away" or "retryable",
		// we should stop parsing the error message.
		return true
	}
	return false
}

// isErrorUnexpectedEOF returns true if error is generated by gRPC layer
// receiving io.EOF unexpectedly.
func isErrorUnexpectedEOF(err error) bool {
	if err == nil {
		return false
	}
	if ErrCode(err) == codes.Unknown && strings.Contains(ErrDesc(err), "unexpected EOF") {
		// Unexpected EOF is an transport layer issue that
		// could be recovered by retries. The most likely
		// scenario is a flaky RecvMsg() call due to network
		// issues.
		// TODO: once gRPC is able to categorize
		// this as retryable error, we should stop parsing the
		// error message here.
		return true
	}
	return false
}

// isErrorUnavailable returns true if the error is about server being unavailable.
func isErrorUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if ErrCode(err) == codes.Unavailable {
		return true
	}
	return false
}

// isRetryable returns true if the Cloud Spanner error being checked is a retryable error.
func isRetryable(err error) bool {
	if isErrorClosing(err) {
		return true
	}
	if isErrorUnexpectedEOF(err) {
		return true
	}
	if isErrorRST(err) {
		return true
	}
	if isErrorUnavailable(err) {
		return true
	}
	return false
}

func (e *Error) ErrCode() codes.Code {
	if e == nil {
		return codes.Unknown
	}
	return e.Code
}
