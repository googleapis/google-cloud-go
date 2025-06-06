// Copyright 2025 Google LLC
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

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.2
// 	protoc        v4.25.7
// source: google/cloud/aiplatform/v1/feature.proto

package aiplatformpb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Only applicable for Vertex AI Legacy Feature Store.
// An enum representing the value type of a feature.
type Feature_ValueType int32

const (
	// The value type is unspecified.
	Feature_VALUE_TYPE_UNSPECIFIED Feature_ValueType = 0
	// Used for Feature that is a boolean.
	Feature_BOOL Feature_ValueType = 1
	// Used for Feature that is a list of boolean.
	Feature_BOOL_ARRAY Feature_ValueType = 2
	// Used for Feature that is double.
	Feature_DOUBLE Feature_ValueType = 3
	// Used for Feature that is a list of double.
	Feature_DOUBLE_ARRAY Feature_ValueType = 4
	// Used for Feature that is INT64.
	Feature_INT64 Feature_ValueType = 9
	// Used for Feature that is a list of INT64.
	Feature_INT64_ARRAY Feature_ValueType = 10
	// Used for Feature that is string.
	Feature_STRING Feature_ValueType = 11
	// Used for Feature that is a list of String.
	Feature_STRING_ARRAY Feature_ValueType = 12
	// Used for Feature that is bytes.
	Feature_BYTES Feature_ValueType = 13
	// Used for Feature that is struct.
	Feature_STRUCT Feature_ValueType = 14
)

// Enum value maps for Feature_ValueType.
var (
	Feature_ValueType_name = map[int32]string{
		0:  "VALUE_TYPE_UNSPECIFIED",
		1:  "BOOL",
		2:  "BOOL_ARRAY",
		3:  "DOUBLE",
		4:  "DOUBLE_ARRAY",
		9:  "INT64",
		10: "INT64_ARRAY",
		11: "STRING",
		12: "STRING_ARRAY",
		13: "BYTES",
		14: "STRUCT",
	}
	Feature_ValueType_value = map[string]int32{
		"VALUE_TYPE_UNSPECIFIED": 0,
		"BOOL":                   1,
		"BOOL_ARRAY":             2,
		"DOUBLE":                 3,
		"DOUBLE_ARRAY":           4,
		"INT64":                  9,
		"INT64_ARRAY":            10,
		"STRING":                 11,
		"STRING_ARRAY":           12,
		"BYTES":                  13,
		"STRUCT":                 14,
	}
)

func (x Feature_ValueType) Enum() *Feature_ValueType {
	p := new(Feature_ValueType)
	*p = x
	return p
}

func (x Feature_ValueType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Feature_ValueType) Descriptor() protoreflect.EnumDescriptor {
	return file_google_cloud_aiplatform_v1_feature_proto_enumTypes[0].Descriptor()
}

func (Feature_ValueType) Type() protoreflect.EnumType {
	return &file_google_cloud_aiplatform_v1_feature_proto_enumTypes[0]
}

func (x Feature_ValueType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Feature_ValueType.Descriptor instead.
func (Feature_ValueType) EnumDescriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_feature_proto_rawDescGZIP(), []int{0, 0}
}

// If the objective in the request is both
// Import Feature Analysis and Snapshot Analysis, this objective could be
// one of them. Otherwise, this objective should be the same as the
// objective in the request.
type Feature_MonitoringStatsAnomaly_Objective int32

const (
	// If it's OBJECTIVE_UNSPECIFIED, monitoring_stats will be empty.
	Feature_MonitoringStatsAnomaly_OBJECTIVE_UNSPECIFIED Feature_MonitoringStatsAnomaly_Objective = 0
	// Stats are generated by Import Feature Analysis.
	Feature_MonitoringStatsAnomaly_IMPORT_FEATURE_ANALYSIS Feature_MonitoringStatsAnomaly_Objective = 1
	// Stats are generated by Snapshot Analysis.
	Feature_MonitoringStatsAnomaly_SNAPSHOT_ANALYSIS Feature_MonitoringStatsAnomaly_Objective = 2
)

// Enum value maps for Feature_MonitoringStatsAnomaly_Objective.
var (
	Feature_MonitoringStatsAnomaly_Objective_name = map[int32]string{
		0: "OBJECTIVE_UNSPECIFIED",
		1: "IMPORT_FEATURE_ANALYSIS",
		2: "SNAPSHOT_ANALYSIS",
	}
	Feature_MonitoringStatsAnomaly_Objective_value = map[string]int32{
		"OBJECTIVE_UNSPECIFIED":   0,
		"IMPORT_FEATURE_ANALYSIS": 1,
		"SNAPSHOT_ANALYSIS":       2,
	}
)

func (x Feature_MonitoringStatsAnomaly_Objective) Enum() *Feature_MonitoringStatsAnomaly_Objective {
	p := new(Feature_MonitoringStatsAnomaly_Objective)
	*p = x
	return p
}

func (x Feature_MonitoringStatsAnomaly_Objective) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Feature_MonitoringStatsAnomaly_Objective) Descriptor() protoreflect.EnumDescriptor {
	return file_google_cloud_aiplatform_v1_feature_proto_enumTypes[1].Descriptor()
}

func (Feature_MonitoringStatsAnomaly_Objective) Type() protoreflect.EnumType {
	return &file_google_cloud_aiplatform_v1_feature_proto_enumTypes[1]
}

func (x Feature_MonitoringStatsAnomaly_Objective) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Feature_MonitoringStatsAnomaly_Objective.Descriptor instead.
func (Feature_MonitoringStatsAnomaly_Objective) EnumDescriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_feature_proto_rawDescGZIP(), []int{0, 0, 0}
}

// Feature Metadata information.
// For example, color is a feature that describes an apple.
type Feature struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Immutable. Name of the Feature.
	// Format:
	// `projects/{project}/locations/{location}/featurestores/{featurestore}/entityTypes/{entity_type}/features/{feature}`
	// `projects/{project}/locations/{location}/featureGroups/{feature_group}/features/{feature}`
	//
	// The last part feature is assigned by the client. The feature can be up to
	// 64 characters long and can consist only of ASCII Latin letters A-Z and a-z,
	// underscore(_), and ASCII digits 0-9 starting with a letter. The value will
	// be unique given an entity type.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Description of the Feature.
	Description string `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	// Immutable. Only applicable for Vertex AI Feature Store (Legacy).
	// Type of Feature value.
	ValueType Feature_ValueType `protobuf:"varint,3,opt,name=value_type,json=valueType,proto3,enum=google.cloud.aiplatform.v1.Feature_ValueType" json:"value_type,omitempty"`
	// Output only. Only applicable for Vertex AI Feature Store (Legacy).
	// Timestamp when this EntityType was created.
	CreateTime *timestamppb.Timestamp `protobuf:"bytes,4,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// Output only. Only applicable for Vertex AI Feature Store (Legacy).
	// Timestamp when this EntityType was most recently updated.
	UpdateTime *timestamppb.Timestamp `protobuf:"bytes,5,opt,name=update_time,json=updateTime,proto3" json:"update_time,omitempty"`
	// Optional. The labels with user-defined metadata to organize your Features.
	//
	// Label keys and values can be no longer than 64 characters
	// (Unicode codepoints), can only contain lowercase letters, numeric
	// characters, underscores and dashes. International characters are allowed.
	//
	// See https://goo.gl/xmQnxf for more information on and examples of labels.
	// No more than 64 user labels can be associated with one Feature (System
	// labels are excluded)."
	// System reserved label keys are prefixed with "aiplatform.googleapis.com/"
	// and are immutable.
	Labels map[string]string `protobuf:"bytes,6,rep,name=labels,proto3" json:"labels,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// Used to perform a consistent read-modify-write updates. If not set, a blind
	// "overwrite" update happens.
	Etag string `protobuf:"bytes,7,opt,name=etag,proto3" json:"etag,omitempty"`
	// Optional. Only applicable for Vertex AI Feature Store (Legacy).
	// If not set, use the monitoring_config defined for the EntityType this
	// Feature belongs to.
	// Only Features with type
	// ([Feature.ValueType][google.cloud.aiplatform.v1.Feature.ValueType]) BOOL,
	// STRING, DOUBLE or INT64 can enable monitoring.
	//
	// If set to true, all types of data monitoring are disabled despite the
	// config on EntityType.
	DisableMonitoring bool `protobuf:"varint,12,opt,name=disable_monitoring,json=disableMonitoring,proto3" json:"disable_monitoring,omitempty"`
	// Output only. Only applicable for Vertex AI Feature Store (Legacy).
	// The list of historical stats and anomalies with specified objectives.
	MonitoringStatsAnomalies []*Feature_MonitoringStatsAnomaly `protobuf:"bytes,11,rep,name=monitoring_stats_anomalies,json=monitoringStatsAnomalies,proto3" json:"monitoring_stats_anomalies,omitempty"`
	// Only applicable for Vertex AI Feature Store.
	// The name of the BigQuery Table/View column hosting data for this version.
	// If no value is provided, will use feature_id.
	VersionColumnName string `protobuf:"bytes,106,opt,name=version_column_name,json=versionColumnName,proto3" json:"version_column_name,omitempty"`
	// Entity responsible for maintaining this feature. Can be comma separated
	// list of email addresses or URIs.
	PointOfContact string `protobuf:"bytes,107,opt,name=point_of_contact,json=pointOfContact,proto3" json:"point_of_contact,omitempty"`
}

func (x *Feature) Reset() {
	*x = Feature{}
	mi := &file_google_cloud_aiplatform_v1_feature_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Feature) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Feature) ProtoMessage() {}

func (x *Feature) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_aiplatform_v1_feature_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Feature.ProtoReflect.Descriptor instead.
func (*Feature) Descriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_feature_proto_rawDescGZIP(), []int{0}
}

func (x *Feature) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Feature) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *Feature) GetValueType() Feature_ValueType {
	if x != nil {
		return x.ValueType
	}
	return Feature_VALUE_TYPE_UNSPECIFIED
}

func (x *Feature) GetCreateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.CreateTime
	}
	return nil
}

func (x *Feature) GetUpdateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.UpdateTime
	}
	return nil
}

func (x *Feature) GetLabels() map[string]string {
	if x != nil {
		return x.Labels
	}
	return nil
}

func (x *Feature) GetEtag() string {
	if x != nil {
		return x.Etag
	}
	return ""
}

func (x *Feature) GetDisableMonitoring() bool {
	if x != nil {
		return x.DisableMonitoring
	}
	return false
}

func (x *Feature) GetMonitoringStatsAnomalies() []*Feature_MonitoringStatsAnomaly {
	if x != nil {
		return x.MonitoringStatsAnomalies
	}
	return nil
}

func (x *Feature) GetVersionColumnName() string {
	if x != nil {
		return x.VersionColumnName
	}
	return ""
}

func (x *Feature) GetPointOfContact() string {
	if x != nil {
		return x.PointOfContact
	}
	return ""
}

// A list of historical
// [SnapshotAnalysis][google.cloud.aiplatform.v1.FeaturestoreMonitoringConfig.SnapshotAnalysis]
// or
// [ImportFeaturesAnalysis][google.cloud.aiplatform.v1.FeaturestoreMonitoringConfig.ImportFeaturesAnalysis]
// stats requested by user, sorted by
// [FeatureStatsAnomaly.start_time][google.cloud.aiplatform.v1.FeatureStatsAnomaly.start_time]
// descending.
type Feature_MonitoringStatsAnomaly struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Output only. The objective for each stats.
	Objective Feature_MonitoringStatsAnomaly_Objective `protobuf:"varint,1,opt,name=objective,proto3,enum=google.cloud.aiplatform.v1.Feature_MonitoringStatsAnomaly_Objective" json:"objective,omitempty"`
	// Output only. The stats and anomalies generated at specific timestamp.
	FeatureStatsAnomaly *FeatureStatsAnomaly `protobuf:"bytes,2,opt,name=feature_stats_anomaly,json=featureStatsAnomaly,proto3" json:"feature_stats_anomaly,omitempty"`
}

func (x *Feature_MonitoringStatsAnomaly) Reset() {
	*x = Feature_MonitoringStatsAnomaly{}
	mi := &file_google_cloud_aiplatform_v1_feature_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Feature_MonitoringStatsAnomaly) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Feature_MonitoringStatsAnomaly) ProtoMessage() {}

func (x *Feature_MonitoringStatsAnomaly) ProtoReflect() protoreflect.Message {
	mi := &file_google_cloud_aiplatform_v1_feature_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Feature_MonitoringStatsAnomaly.ProtoReflect.Descriptor instead.
func (*Feature_MonitoringStatsAnomaly) Descriptor() ([]byte, []int) {
	return file_google_cloud_aiplatform_v1_feature_proto_rawDescGZIP(), []int{0, 0}
}

func (x *Feature_MonitoringStatsAnomaly) GetObjective() Feature_MonitoringStatsAnomaly_Objective {
	if x != nil {
		return x.Objective
	}
	return Feature_MonitoringStatsAnomaly_OBJECTIVE_UNSPECIFIED
}

func (x *Feature_MonitoringStatsAnomaly) GetFeatureStatsAnomaly() *FeatureStatsAnomaly {
	if x != nil {
		return x.FeatureStatsAnomaly
	}
	return nil
}

var File_google_cloud_aiplatform_v1_feature_proto protoreflect.FileDescriptor

var file_google_cloud_aiplatform_v1_feature_proto_rawDesc = []byte{
	0x0a, 0x28, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2f, 0x61,
	0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2f, 0x76, 0x31, 0x2f, 0x66, 0x65, 0x61,
	0x74, 0x75, 0x72, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x1a, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66,
	0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61,
	0x70, 0x69, 0x2f, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f,
	0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f,
	0x61, 0x70, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x1a, 0x39, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64,
	0x2f, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2f, 0x76, 0x31, 0x2f, 0x66,
	0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x5f, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x69, 0x6e,
	0x67, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74,
	0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xcc,
	0x0b, 0x0a, 0x07, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x12, 0x17, 0x0a, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x05, 0x52, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x51, 0x0a, 0x0a, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x5f, 0x74,
	0x79, 0x70, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x2d, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66,
	0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x2e, 0x56,
	0x61, 0x6c, 0x75, 0x65, 0x54, 0x79, 0x70, 0x65, 0x42, 0x03, 0xe0, 0x41, 0x05, 0x52, 0x09, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x40, 0x0a, 0x0b, 0x63, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0a,
	0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x40, 0x0a, 0x0b, 0x75, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41, 0x03,
	0x52, 0x0a, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x4c, 0x0a, 0x06,
	0x6c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2f, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c,
	0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72,
	0x65, 0x2e, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x42, 0x03, 0xe0,
	0x41, 0x01, 0x52, 0x06, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x12, 0x12, 0x0a, 0x04, 0x65, 0x74,
	0x61, 0x67, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x65, 0x74, 0x61, 0x67, 0x12, 0x32,
	0x0a, 0x12, 0x64, 0x69, 0x73, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f,
	0x72, 0x69, 0x6e, 0x67, 0x18, 0x0c, 0x20, 0x01, 0x28, 0x08, 0x42, 0x03, 0xe0, 0x41, 0x01, 0x52,
	0x11, 0x64, 0x69, 0x73, 0x61, 0x62, 0x6c, 0x65, 0x4d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x69,
	0x6e, 0x67, 0x12, 0x7d, 0x0a, 0x1a, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x69, 0x6e, 0x67,
	0x5f, 0x73, 0x74, 0x61, 0x74, 0x73, 0x5f, 0x61, 0x6e, 0x6f, 0x6d, 0x61, 0x6c, 0x69, 0x65, 0x73,
	0x18, 0x0b, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x3a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x2e, 0x76, 0x31, 0x2e, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x2e, 0x4d, 0x6f, 0x6e, 0x69,
	0x74, 0x6f, 0x72, 0x69, 0x6e, 0x67, 0x53, 0x74, 0x61, 0x74, 0x73, 0x41, 0x6e, 0x6f, 0x6d, 0x61,
	0x6c, 0x79, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x18, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72,
	0x69, 0x6e, 0x67, 0x53, 0x74, 0x61, 0x74, 0x73, 0x41, 0x6e, 0x6f, 0x6d, 0x61, 0x6c, 0x69, 0x65,
	0x73, 0x12, 0x2e, 0x0a, 0x13, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x6f, 0x6c,
	0x75, 0x6d, 0x6e, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x6a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x11,
	0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x6c, 0x75, 0x6d, 0x6e, 0x4e, 0x61, 0x6d,
	0x65, 0x12, 0x28, 0x0a, 0x10, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x5f, 0x6f, 0x66, 0x5f, 0x63, 0x6f,
	0x6e, 0x74, 0x61, 0x63, 0x74, 0x18, 0x6b, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x70, 0x6f, 0x69,
	0x6e, 0x74, 0x4f, 0x66, 0x43, 0x6f, 0x6e, 0x74, 0x61, 0x63, 0x74, 0x1a, 0xc7, 0x02, 0x0a, 0x16,
	0x4d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x69, 0x6e, 0x67, 0x53, 0x74, 0x61, 0x74, 0x73, 0x41,
	0x6e, 0x6f, 0x6d, 0x61, 0x6c, 0x79, 0x12, 0x67, 0x0a, 0x09, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74,
	0x69, 0x76, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x44, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66,
	0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x2e, 0x4d,
	0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x69, 0x6e, 0x67, 0x53, 0x74, 0x61, 0x74, 0x73, 0x41, 0x6e,
	0x6f, 0x6d, 0x61, 0x6c, 0x79, 0x2e, 0x4f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x69, 0x76, 0x65, 0x42,
	0x03, 0xe0, 0x41, 0x03, 0x52, 0x09, 0x6f, 0x62, 0x6a, 0x65, 0x63, 0x74, 0x69, 0x76, 0x65, 0x12,
	0x68, 0x0a, 0x15, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x73,
	0x5f, 0x61, 0x6e, 0x6f, 0x6d, 0x61, 0x6c, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x2f,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x61, 0x69,
	0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x65, 0x61, 0x74,
	0x75, 0x72, 0x65, 0x53, 0x74, 0x61, 0x74, 0x73, 0x41, 0x6e, 0x6f, 0x6d, 0x61, 0x6c, 0x79, 0x42,
	0x03, 0xe0, 0x41, 0x03, 0x52, 0x13, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x53, 0x74, 0x61,
	0x74, 0x73, 0x41, 0x6e, 0x6f, 0x6d, 0x61, 0x6c, 0x79, 0x22, 0x5a, 0x0a, 0x09, 0x4f, 0x62, 0x6a,
	0x65, 0x63, 0x74, 0x69, 0x76, 0x65, 0x12, 0x19, 0x0a, 0x15, 0x4f, 0x42, 0x4a, 0x45, 0x43, 0x54,
	0x49, 0x56, 0x45, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10,
	0x00, 0x12, 0x1b, 0x0a, 0x17, 0x49, 0x4d, 0x50, 0x4f, 0x52, 0x54, 0x5f, 0x46, 0x45, 0x41, 0x54,
	0x55, 0x52, 0x45, 0x5f, 0x41, 0x4e, 0x41, 0x4c, 0x59, 0x53, 0x49, 0x53, 0x10, 0x01, 0x12, 0x15,
	0x0a, 0x11, 0x53, 0x4e, 0x41, 0x50, 0x53, 0x48, 0x4f, 0x54, 0x5f, 0x41, 0x4e, 0x41, 0x4c, 0x59,
	0x53, 0x49, 0x53, 0x10, 0x02, 0x1a, 0x39, 0x0a, 0x0b, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x73, 0x45,
	0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01,
	0x22, 0xb0, 0x01, 0x0a, 0x09, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1a,
	0x0a, 0x16, 0x56, 0x41, 0x4c, 0x55, 0x45, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x5f, 0x55, 0x4e, 0x53,
	0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x08, 0x0a, 0x04, 0x42, 0x4f,
	0x4f, 0x4c, 0x10, 0x01, 0x12, 0x0e, 0x0a, 0x0a, 0x42, 0x4f, 0x4f, 0x4c, 0x5f, 0x41, 0x52, 0x52,
	0x41, 0x59, 0x10, 0x02, 0x12, 0x0a, 0x0a, 0x06, 0x44, 0x4f, 0x55, 0x42, 0x4c, 0x45, 0x10, 0x03,
	0x12, 0x10, 0x0a, 0x0c, 0x44, 0x4f, 0x55, 0x42, 0x4c, 0x45, 0x5f, 0x41, 0x52, 0x52, 0x41, 0x59,
	0x10, 0x04, 0x12, 0x09, 0x0a, 0x05, 0x49, 0x4e, 0x54, 0x36, 0x34, 0x10, 0x09, 0x12, 0x0f, 0x0a,
	0x0b, 0x49, 0x4e, 0x54, 0x36, 0x34, 0x5f, 0x41, 0x52, 0x52, 0x41, 0x59, 0x10, 0x0a, 0x12, 0x0a,
	0x0a, 0x06, 0x53, 0x54, 0x52, 0x49, 0x4e, 0x47, 0x10, 0x0b, 0x12, 0x10, 0x0a, 0x0c, 0x53, 0x54,
	0x52, 0x49, 0x4e, 0x47, 0x5f, 0x41, 0x52, 0x52, 0x41, 0x59, 0x10, 0x0c, 0x12, 0x09, 0x0a, 0x05,
	0x42, 0x59, 0x54, 0x45, 0x53, 0x10, 0x0d, 0x12, 0x0a, 0x0a, 0x06, 0x53, 0x54, 0x52, 0x55, 0x43,
	0x54, 0x10, 0x0e, 0x3a, 0x87, 0x02, 0xea, 0x41, 0x83, 0x02, 0x0a, 0x21, 0x61, 0x69, 0x70, 0x6c,
	0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x61, 0x70, 0x69,
	0x73, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x12, 0x71, 0x70,
	0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x7b, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x7d, 0x2f, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x7b, 0x6c, 0x6f, 0x63,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x7d, 0x2f, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x73, 0x74,
	0x6f, 0x72, 0x65, 0x73, 0x2f, 0x7b, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x7d, 0x2f, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x54, 0x79, 0x70, 0x65, 0x73, 0x2f,
	0x7b, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x7d, 0x2f, 0x66, 0x65,
	0x61, 0x74, 0x75, 0x72, 0x65, 0x73, 0x2f, 0x7b, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x7d,
	0x12, 0x58, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73, 0x2f, 0x7b, 0x70, 0x72, 0x6f, 0x6a,
	0x65, 0x63, 0x74, 0x7d, 0x2f, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x7b,
	0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x7d, 0x2f, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72,
	0x65, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x73, 0x2f, 0x7b, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65,
	0x5f, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x7d, 0x2f, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x73,
	0x2f, 0x7b, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x7d, 0x2a, 0x08, 0x66, 0x65, 0x61, 0x74,
	0x75, 0x72, 0x65, 0x73, 0x32, 0x07, 0x66, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x42, 0xca, 0x01,
	0x0a, 0x1e, 0x63, 0x6f, 0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63, 0x6c, 0x6f,
	0x75, 0x64, 0x2e, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x76, 0x31,
	0x42, 0x0c, 0x46, 0x65, 0x61, 0x74, 0x75, 0x72, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01,
	0x5a, 0x3e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x2f, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d,
	0x2f, 0x61, 0x70, 0x69, 0x76, 0x31, 0x2f, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72,
	0x6d, 0x70, 0x62, 0x3b, 0x61, 0x69, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x70, 0x62,
	0xaa, 0x02, 0x1a, 0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x2e,
	0x41, 0x49, 0x50, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x2e, 0x56, 0x31, 0xca, 0x02, 0x1a,
	0x47, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x5c, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x5c, 0x41, 0x49, 0x50,
	0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x5c, 0x56, 0x31, 0xea, 0x02, 0x1d, 0x47, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x3a, 0x3a, 0x43, 0x6c, 0x6f, 0x75, 0x64, 0x3a, 0x3a, 0x41, 0x49, 0x50, 0x6c,
	0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_google_cloud_aiplatform_v1_feature_proto_rawDescOnce sync.Once
	file_google_cloud_aiplatform_v1_feature_proto_rawDescData = file_google_cloud_aiplatform_v1_feature_proto_rawDesc
)

func file_google_cloud_aiplatform_v1_feature_proto_rawDescGZIP() []byte {
	file_google_cloud_aiplatform_v1_feature_proto_rawDescOnce.Do(func() {
		file_google_cloud_aiplatform_v1_feature_proto_rawDescData = protoimpl.X.CompressGZIP(file_google_cloud_aiplatform_v1_feature_proto_rawDescData)
	})
	return file_google_cloud_aiplatform_v1_feature_proto_rawDescData
}

var file_google_cloud_aiplatform_v1_feature_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_google_cloud_aiplatform_v1_feature_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_google_cloud_aiplatform_v1_feature_proto_goTypes = []any{
	(Feature_ValueType)(0),                        // 0: google.cloud.aiplatform.v1.Feature.ValueType
	(Feature_MonitoringStatsAnomaly_Objective)(0), // 1: google.cloud.aiplatform.v1.Feature.MonitoringStatsAnomaly.Objective
	(*Feature)(nil),                               // 2: google.cloud.aiplatform.v1.Feature
	(*Feature_MonitoringStatsAnomaly)(nil),        // 3: google.cloud.aiplatform.v1.Feature.MonitoringStatsAnomaly
	nil,                                           // 4: google.cloud.aiplatform.v1.Feature.LabelsEntry
	(*timestamppb.Timestamp)(nil),                 // 5: google.protobuf.Timestamp
	(*FeatureStatsAnomaly)(nil),                   // 6: google.cloud.aiplatform.v1.FeatureStatsAnomaly
}
var file_google_cloud_aiplatform_v1_feature_proto_depIdxs = []int32{
	0, // 0: google.cloud.aiplatform.v1.Feature.value_type:type_name -> google.cloud.aiplatform.v1.Feature.ValueType
	5, // 1: google.cloud.aiplatform.v1.Feature.create_time:type_name -> google.protobuf.Timestamp
	5, // 2: google.cloud.aiplatform.v1.Feature.update_time:type_name -> google.protobuf.Timestamp
	4, // 3: google.cloud.aiplatform.v1.Feature.labels:type_name -> google.cloud.aiplatform.v1.Feature.LabelsEntry
	3, // 4: google.cloud.aiplatform.v1.Feature.monitoring_stats_anomalies:type_name -> google.cloud.aiplatform.v1.Feature.MonitoringStatsAnomaly
	1, // 5: google.cloud.aiplatform.v1.Feature.MonitoringStatsAnomaly.objective:type_name -> google.cloud.aiplatform.v1.Feature.MonitoringStatsAnomaly.Objective
	6, // 6: google.cloud.aiplatform.v1.Feature.MonitoringStatsAnomaly.feature_stats_anomaly:type_name -> google.cloud.aiplatform.v1.FeatureStatsAnomaly
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	7, // [7:7] is the sub-list for extension extendee
	0, // [0:7] is the sub-list for field type_name
}

func init() { file_google_cloud_aiplatform_v1_feature_proto_init() }
func file_google_cloud_aiplatform_v1_feature_proto_init() {
	if File_google_cloud_aiplatform_v1_feature_proto != nil {
		return
	}
	file_google_cloud_aiplatform_v1_feature_monitoring_stats_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_google_cloud_aiplatform_v1_feature_proto_rawDesc,
			NumEnums:      2,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_google_cloud_aiplatform_v1_feature_proto_goTypes,
		DependencyIndexes: file_google_cloud_aiplatform_v1_feature_proto_depIdxs,
		EnumInfos:         file_google_cloud_aiplatform_v1_feature_proto_enumTypes,
		MessageInfos:      file_google_cloud_aiplatform_v1_feature_proto_msgTypes,
	}.Build()
	File_google_cloud_aiplatform_v1_feature_proto = out.File
	file_google_cloud_aiplatform_v1_feature_proto_rawDesc = nil
	file_google_cloud_aiplatform_v1_feature_proto_goTypes = nil
	file_google_cloud_aiplatform_v1_feature_proto_depIdxs = nil
}
