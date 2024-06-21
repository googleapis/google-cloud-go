// Copyright 2023 Google LLC
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

package utility

import (
	"math/big"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KeySetProtoToCloudKeySet converts executorpb.KeySet to spanner.KeySet.
func KeySetProtoToCloudKeySet(keySetProto *executorpb.KeySet, typeList []*spannerpb.Type) (spanner.KeySet, error) {
	if keySetProto.GetAll() {
		return spanner.AllKeys(), nil
	}
	cloudKeySet := spanner.KeySets()
	for _, techKey := range keySetProto.GetPoint() {
		cloudKey, err := keyProtoToCloudKey(techKey, typeList)
		if err != nil {
			return nil, err
		}
		cloudKeySet = spanner.KeySets(cloudKey, cloudKeySet)
	}
	for _, techRange := range keySetProto.GetRange() {
		cloudRange, err := keyRangeProtoToCloudKeyRange(techRange, typeList)
		if err != nil {
			return nil, err
		}
		cloudKeySet = spanner.KeySets(cloudKeySet, cloudRange)
	}
	return cloudKeySet, nil
}

// keyProtoToCloudKey converts executorpb.ValueList to spanner.Key.
func keyProtoToCloudKey(keyProto *executorpb.ValueList, typeList []*spannerpb.Type) (spanner.Key, error) {
	if len(typeList) < len(keyProto.GetValue()) {
		return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "There's more key parts in %s than column types in %s", keyProto, typeList))
	}

	var cloudKey spanner.Key
	for i, part := range keyProto.GetValue() {
		typePb := typeList[i]
		key, err := executorKeyValueToCloudValue(part, typePb)
		if err != nil {
			return nil, err
		}
		cloudKey = append(cloudKey, key)
	}
	return cloudKey, nil
}

// executorKeyValueToCloudValue converts executorpb.Value of the given type to an interface value suitable for
// Cloud Spanner API.
func executorKeyValueToCloudValue(part *executorpb.Value, typePb *spannerpb.Type) (any, error) {
	switch v := part.ValueType.(type) {
	// Check the value type
	case *executorpb.Value_IsNull:
		// Check the column type if the value is nil.
		switch typePb.GetCode() {
		case sppb.TypeCode_BOOL:
			return spanner.NullBool{}, nil
		case sppb.TypeCode_INT64:
			return spanner.NullInt64{}, nil
		case sppb.TypeCode_STRING:
			return spanner.NullString{}, nil
		case sppb.TypeCode_BYTES:
			return []byte(nil), nil
		case sppb.TypeCode_FLOAT64:
			return spanner.NullFloat64{}, nil
		case sppb.TypeCode_DATE:
			return spanner.NullDate{}, nil
		case sppb.TypeCode_TIMESTAMP:
			return spanner.NullTime{}, nil
		case sppb.TypeCode_NUMERIC:
			return spanner.NullNumeric{}, nil
		case sppb.TypeCode_JSON:
			return spanner.NullJSON{}, nil
		default:
			return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Unsupported null key part type: %s", typePb.GetCode().String()))
		}
	case *executorpb.Value_IntValue:
		return v.IntValue, nil
	case *executorpb.Value_BoolValue:
		return v.BoolValue, nil
	case *executorpb.Value_DoubleValue:
		return v.DoubleValue, nil
	case *executorpb.Value_BytesValue:
		switch typePb.GetCode() {
		case sppb.TypeCode_STRING:
			return string(v.BytesValue), nil
		case sppb.TypeCode_BYTES:
			return v.BytesValue, nil
		default:
			return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Unsupported key part type: %s", typePb.GetCode().String()))
		}
	case *executorpb.Value_StringValue:
		switch typePb.GetCode() {
		case sppb.TypeCode_NUMERIC:
			y, ok := (&big.Rat{}).SetString(v.StringValue)
			if !ok {
				return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Unexpected string value %q for numeric number", v.StringValue))
			}
			return *y, nil
		default:
			return v.StringValue, nil
		}
	case *executorpb.Value_TimestampValue:
		if err := v.TimestampValue.CheckValid(); err != nil {
			return nil, err
		}
		return v.TimestampValue.AsTime(), nil
	case *executorpb.Value_DateDaysValue:
		epoch := civil.DateOf(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		y := epoch.AddDays(int(v.DateDaysValue))
		return y, nil
	}
	return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Unsupported key part %s with type %s", part, typePb))
}

// keyRangeProtoToCloudKeyRange converts executorpb.KeyRange to spanner.KeyRange.
func keyRangeProtoToCloudKeyRange(keyRangeProto *executorpb.KeyRange, typeList []*spannerpb.Type) (spanner.KeyRange, error) {
	start, err := keyProtoToCloudKey(keyRangeProto.GetStart(), typeList)
	if err != nil {
		return spanner.KeyRange{}, err
	}
	end, err := keyProtoToCloudKey(keyRangeProto.GetLimit(), typeList)
	if err != nil {
		return spanner.KeyRange{}, err
	}
	if keyRangeProto.Type == nil {
		// default
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.ClosedOpen}, nil
	}
	switch keyRangeProto.GetType() {
	case executorpb.KeyRange_CLOSED_CLOSED:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.ClosedClosed}, nil
	case executorpb.KeyRange_CLOSED_OPEN:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.ClosedOpen}, nil
	case executorpb.KeyRange_OPEN_CLOSED:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.OpenClosed}, nil
	case executorpb.KeyRange_OPEN_OPEN:
		return spanner.KeyRange{Start: start, End: end, Kind: spanner.OpenOpen}, nil
	default:
		return spanner.KeyRange{}, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "Unrecognized key range type %s", keyRangeProto.GetType().String()))
	}
}
