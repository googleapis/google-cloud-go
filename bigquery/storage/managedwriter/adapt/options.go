// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adapt

import (
	"strings"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Option to customize proto descriptor conversion.
type Option interface {
	applyCustomClientOpt(*customConfig)
}

// type for collecting custom adapt Option values.
type customConfig struct {
	protoMappingOverrides map[storagepb.TableFieldSchema_Type]protoOverride
	useProto3             bool
}

type protoOverride struct {
	fieldType storagepb.TableFieldSchema_Type
	typeName  string
	protoType descriptorpb.FieldDescriptorProto_Type
}

type customOption struct {
	protoOverride *protoOverride
}

// WithTimestampAsTimestamp defines that table fields of type Timestamp, are mapped
// as Google's WKT timestamppb.Timestamp.
// THIS IS GOING TO BE REMOVED
func WithTimestampAsTimestamp() Option {
	return WithProtoMapping(storagepb.TableFieldSchema_TIMESTAMP, "google.protobuf.Timestamp", descriptorpb.FieldDescriptorProto_TYPE_MESSAGE)
}

// WithIntervalAsDuration defines that table fields of type Interval, are mapped
// as Google's WKT durationpb.Duration
// THIS IS GOING TO BE REMOVED
func WithIntervalAsDuration() Option {
	return WithProtoMapping(storagepb.TableFieldSchema_INTERVAL, "google.protobuf.Duration", descriptorpb.FieldDescriptorProto_TYPE_MESSAGE)
}

// WithBigNumericAsDouble defines that table fields of type BigNumeric, are mapped
// as Google's WKT wrapperspb.Double
// THIS IS GOING TO BE REMOVED
func WithBigNumericAsDouble() Option {
	return WithProtoMapping(storagepb.TableFieldSchema_BIGNUMERIC, "google.protobuf.DoubleValue", descriptorpb.FieldDescriptorProto_TYPE_DOUBLE)
}

// WithProtoMapping overrides which field descriptor proto type is going to be used
// for the given table field type.
func WithProtoMapping(fieldType storagepb.TableFieldSchema_Type, typeName string, protoType descriptorpb.FieldDescriptorProto_Type) Option {
	if !strings.HasPrefix(typeName, ".") {
		typeName = "." + typeName
	}
	return &customOption{protoOverride: &protoOverride{
		fieldType: fieldType,
		typeName:  typeName,
		protoType: protoType,
	}}
}

func (o *customOption) applyCustomClientOpt(cfg *customConfig) {
	if o.protoOverride != nil {
		cfg.protoMappingOverrides[o.protoOverride.fieldType] = *o.protoOverride
	}
}
