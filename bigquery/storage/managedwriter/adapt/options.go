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
	"cloud.google.com/go/internal/optional"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ProtoConversionOption to customize proto descriptor conversion.
type ProtoConversionOption interface {
	applyCustomClientOpt(*customConfig)
}

// type for collecting custom adapt Option values.
type customConfig struct {
	protoMappingOverrides protoMappingOverrides
	useProto3             bool
}

// ProtoMapping can be used to override protobuf types used when
// converting from a BigQuery Schema to a Protobuf Descriptor.
// See [WithProtoMapping] option.
type ProtoMapping struct {
	// FieldPath should be in the `fieldA.subFieldB.anotherSubFieldC`.
	FieldPath string
	// FieldType is the BigQuery Table field type to be overrided
	FieldType storagepb.TableFieldSchema_Type
	// TypeName is the full qualified path name for the protobuf type.
	// Example: ".google.protobuf.Timestamp", ".google.protobuf.Duration", etc
	TypeName string
	// Type is the final DescriptorProto Type
	Type descriptorpb.FieldDescriptorProto_Type
}

type protoMappingOverrides []ProtoMapping

func (o *protoMappingOverrides) getByField(field *storagepb.TableFieldSchema, path string) *ProtoMapping {
	var foundOverride *ProtoMapping
	for _, override := range *o {
		if override.FieldPath == path { // only early return for specific override by path
			return &override
		}
		if override.FieldType == field.Type {
			foundOverride = &override
		}
	}
	return foundOverride
}

type customOption struct {
	protoOverride *ProtoMapping
	useProto3     optional.Bool
}

// WithProtoMapping allow to set an override on which field descriptor proto type
// is going to be used for the given BigQuery Table field type or field path.
// See https://cloud.google.com/bigquery/docs/supported-data-types#supported_protocol_buffer_data_types for accepted types
// by the BigQuery Storage Write API.
//
// Examples:
//
//	// WithTimestampAsTimestamp defines that table fields of type Timestamp, are mapped
//	// as Google's WKT timestamppb.Timestamp.
//	func WithTimestampAsTimestamp() Option {
//		return WithProtoMapping(ProtoMapping{
//			FieldType: storagepb.TableFieldSchema_TIMESTAMP,
//			TypeName:  "google.protobuf.Timestamp",
//			Type:      descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
//		})
//	}
//
//	// WithIntervalAsDuration defines that table fields of type Interval, are mapped
//	// as Google's WKT durationpb.Duration
//	func WithIntervalAsDuration() Option {
//		return WithProtoMapping(ProtoMapping{
//			FieldType: storagepb.TableFieldSchema_INTERVAL,
//			TypeName:  "google.protobuf.Duration",
//			Type:      descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
//		})
//	}
func WithProtoMapping(protoMapping ProtoMapping) ProtoConversionOption {
	if !strings.HasPrefix(protoMapping.TypeName, ".") && protoMapping.TypeName != "" {
		protoMapping.TypeName = "." + protoMapping.TypeName
	}
	return &customOption{protoOverride: &protoMapping}
}

// internal option to set proto 2 syntax option
func withProto2() ProtoConversionOption {
	return &customOption{useProto3: false}
}

// internal option to set proto 3 syntax option
func withProto3() ProtoConversionOption {
	return &customOption{useProto3: true}
}

func (o *customOption) applyCustomClientOpt(cfg *customConfig) {
	if o.protoOverride != nil {
		cfg.protoMappingOverrides = append(cfg.protoMappingOverrides, *o.protoOverride)
	}
	if o.useProto3 != nil {
		cfg.useProto3 = optional.ToBool(o.useProto3)
	}
}
