// Copyright 2025 Google LLC
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

package storage

import (
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	raw "google.golang.org/api/storage/v1"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestToObjectContexts(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		raw  *raw.ObjectContexts
		want *ObjectContexts
	}{
		{
			name: "nil raw object contexts",
			raw:  nil,
			want: nil,
		},
		{
			name: "empty custom contexts",
			raw: &raw.ObjectContexts{
				Custom: map[string]raw.ObjectCustomContextPayload{},
			},
			want: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{},
			},
		},
		{
			name: "with custom contexts",
			raw: &raw.ObjectContexts{
				Custom: map[string]raw.ObjectCustomContextPayload{
					"key1": {Value: "value1", CreateTime: now.Format(time.RFC3339Nano), UpdateTime: now.Format(time.RFC3339Nano)},
					"key2": {Value: "value2", CreateTime: now.Format(time.RFC3339Nano), UpdateTime: now.Format(time.RFC3339Nano)},
				},
			},
			want: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{
					"key1": {Value: "value1", CreateTime: now, UpdateTime: now},
					"key2": {Value: "value2", CreateTime: now, UpdateTime: now},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := toObjectContexts(tc.raw)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("toObjectContexts() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToRawObjectContexts(t *testing.T) {
	testCases := []struct {
		name string
		obj  *ObjectContexts
		want *raw.ObjectContexts
	}{
		{
			name: "nil object contexts",
			obj:  nil,
			want: nil,
		},
		{
			name: "empty custom contexts",
			obj: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{},
			},
			want: &raw.ObjectContexts{
				Custom: map[string]raw.ObjectCustomContextPayload{},
			},
		},
		{
			name: "with custom contexts",
			obj: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{
					"key1": {Value: "value1"},
					"key2": {Value: "value2", Delete: true}, // Should have NullFields
				},
			},
			want: &raw.ObjectContexts{
				Custom: map[string]raw.ObjectCustomContextPayload{
					"key1": {Value: "value1", ForceSendFields: []string{"key1"}},
					"key2": {NullFields: []string{"key2"}},
				},
			},
		},
		{
			name: "with custom contexts, no delete",
			obj: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{
					"key1": {Value: "value1"},
					"key2": {Value: "value2"},
				},
			},
			want: &raw.ObjectContexts{
				Custom: map[string]raw.ObjectCustomContextPayload{
					"key1": {Value: "value1", ForceSendFields: []string{"key1"}},
					"key2": {Value: "value2", ForceSendFields: []string{"key2"}},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := toRawObjectContexts(tc.obj)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("toRawObjectContexts() mismatch (-want +got): %s", diff)
			}
		})
	}
}

func TestToObjectContextsFromProto(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name  string
		proto *storagepb.ObjectContexts
		want  *ObjectContexts
	}{
		{
			name:  "nil proto object contexts",
			proto: nil,
			want:  nil,
		},
		{
			name: "empty custom contexts",
			proto: &storagepb.ObjectContexts{
				Custom: map[string]*storagepb.ObjectCustomContextPayload{},
			},
			want: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{},
			},
		},
		{
			name: "with custom contexts",
			proto: &storagepb.ObjectContexts{
				Custom: map[string]*storagepb.ObjectCustomContextPayload{
					"key1": {Value: "value1", CreateTime: timestamppb.New(now), UpdateTime: timestamppb.New(now)},
					"key2": {Value: "value2", CreateTime: timestamppb.New(now), UpdateTime: timestamppb.New(now)},
				},
			},
			want: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{
					"key1": {Value: "value1", CreateTime: now, UpdateTime: now},
					"key2": {Value: "value2", CreateTime: now, UpdateTime: now},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := toObjectContextsFromProto(tc.proto)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("toObjectContextsFromProto() mismatch (-want +got): %s", diff)
			}
		})
	}
}

func TestToProtoObjectContexts(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		obj  *ObjectContexts
		want *storagepb.ObjectContexts
	}{
		{
			name: "nil object contexts",
			obj:  nil,
			want: nil,
		},
		{
			name: "empty custom contexts",
			obj: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{},
			},
			want: &storagepb.ObjectContexts{
				Custom: map[string]*storagepb.ObjectCustomContextPayload{},
			},
		},
		{
			name: "with custom contexts",
			obj: &ObjectContexts{
				Custom: map[string]ObjectCustomContextPayload{
					"key1": {Value: "value1", CreateTime: now, UpdateTime: now},
					"key2": {Value: "value2", Delete: true}, // Should be skipped in proto conversion
					"key3": {Value: "value3"},
				},
			},
			want: &storagepb.ObjectContexts{
				Custom: map[string]*storagepb.ObjectCustomContextPayload{
					"key1": {Value: "value1"},
					"key3": {Value: "value3"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoObjectContexts(tc.obj)
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("toProtoObjectContexts() mismatch (-want +got): %s", diff)
			}
		})
	}
}
