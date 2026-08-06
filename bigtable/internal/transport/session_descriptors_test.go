// Copyright 2026 Google LLC
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

package internal

import (
	"testing"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

func TestSessionType_String(t *testing.T) {
	tests := []struct {
		t    SessionType
		want string
	}{
		{SessionTypeTable, "table"},
		{SessionTypeAuthorizedView, "authorized_view"},
		{SessionTypeMaterializedView, "materialized_view"},
		{SessionType(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.t.String(); got != tt.want {
			t.Errorf("SessionType(%d).String() = %q, want %q", int(tt.t), got, tt.want)
		}
	}
}

func TestTableSessionDescriptor(t *testing.T) {
	desc := TABLE_SESSION

	if desc.Type != SessionTypeTable {
		t.Errorf("Expected type %v, got %v", SessionTypeTable, desc.Type)
	}
	if desc.MethodName != "OpenTable" {
		t.Errorf("Expected MethodName 'OpenTable', got %q", desc.MethodName)
	}

	expectedHeaders := []string{"table_name", "app_profile_id", "permission"}
	if len(desc.HeaderKeys) != len(expectedHeaders) {
		t.Fatalf("Expected HeaderKeys length %d, got %d", len(expectedHeaders), len(desc.HeaderKeys))
	}
	for i, h := range expectedHeaders {
		if desc.HeaderKeys[i] != h {
			t.Errorf("At index %d: expected header key %q, got %q", i, h, desc.HeaderKeys[i])
		}
	}

	req := &spb.OpenTableRequest{
		TableName:    "projects/p/instances/i/tables/t",
		AppProfileId: "app-profile-1",
		Permission:   spb.OpenTableRequest_PERMISSION_READ_WRITE,
	}

	// Test LogNameFn
	logName := desc.LogNameFn(req)
	expectedLogName := "TableSession(table=projects/p/instances/i/tables/t, app_profile=app-profile-1, perm=PERMISSION_READ_WRITE)"
	if logName != expectedLogName {
		t.Errorf("Expected log name %q, got %q", expectedLogName, logName)
	}

	// Test MetadataFn
	meta := desc.MetadataFn(req)
	expectedMeta := map[string]string{
		"open_session.payload.table_name":     "projects/p/instances/i/tables/t",
		"open_session.payload.app_profile_id": "app-profile-1",
		"open_session.payload.permission":     "PERMISSION_READ_WRITE",
	}
	if len(meta) != len(expectedMeta) {
		t.Fatalf("Expected metadata length %d, got %d", len(expectedMeta), len(meta))
	}
	for k, v := range expectedMeta {
		if meta[k] != v {
			t.Errorf("Metadata key %q: expected value %q, got %q", k, v, meta[k])
		}
	}
}

func TestAuthorizedViewSessionDescriptor(t *testing.T) {
	desc := AUTHORIZED_VIEW_SESSION

	if desc.Type != SessionTypeAuthorizedView {
		t.Errorf("Expected type %v, got %v", SessionTypeAuthorizedView, desc.Type)
	}
	if desc.MethodName != "OpenAuthorizedView" {
		t.Errorf("Expected MethodName 'OpenAuthorizedView', got %q", desc.MethodName)
	}

	expectedHeaders := []string{"authorized_view_name", "app_profile_id", "permission"}
	if len(desc.HeaderKeys) != len(expectedHeaders) {
		t.Fatalf("Expected HeaderKeys length %d, got %d", len(expectedHeaders), len(desc.HeaderKeys))
	}
	for i, h := range expectedHeaders {
		if desc.HeaderKeys[i] != h {
			t.Errorf("At index %d: expected header key %q, got %q", i, h, desc.HeaderKeys[i])
		}
	}

	req := &spb.OpenAuthorizedViewRequest{
		AuthorizedViewName: "projects/p/instances/i/tables/t/authorizedViews/v",
		AppProfileId:       "app-profile-2",
		Permission:         spb.OpenAuthorizedViewRequest_PERMISSION_READ,
	}

	// Test LogNameFn
	logName := desc.LogNameFn(req)
	expectedLogName := "AuthorizedViewSession(view=projects/p/instances/i/tables/t/authorizedViews/v, app_profile=app-profile-2, perm=PERMISSION_READ)"
	if logName != expectedLogName {
		t.Errorf("Expected log name %q, got %q", expectedLogName, logName)
	}

	// Test MetadataFn
	meta := desc.MetadataFn(req)
	expectedMeta := map[string]string{
		"open_session.payload.authorized_view_name": "projects/p/instances/i/tables/t/authorizedViews/v",
		"open_session.payload.app_profile_id":       "app-profile-2",
		"open_session.payload.permission":           "PERMISSION_READ",
	}
	if len(meta) != len(expectedMeta) {
		t.Fatalf("Expected metadata length %d, got %d", len(expectedMeta), len(meta))
	}
	for k, v := range expectedMeta {
		if meta[k] != v {
			t.Errorf("Metadata key %q: expected value %q, got %q", k, v, meta[k])
		}
	}
}

func TestMaterializedViewSessionDescriptor(t *testing.T) {
	desc := MATERIALIZED_VIEW_SESSION

	if desc.Type != SessionTypeMaterializedView {
		t.Errorf("Expected type %v, got %v", SessionTypeMaterializedView, desc.Type)
	}
	if desc.MethodName != "OpenMaterializedView" {
		t.Errorf("Expected MethodName 'OpenMaterializedView', got %q", desc.MethodName)
	}

	expectedHeaders := []string{"materialized_view_name", "app_profile_id", "permission"}
	if len(desc.HeaderKeys) != len(expectedHeaders) {
		t.Fatalf("Expected HeaderKeys length %d, got %d", len(expectedHeaders), len(desc.HeaderKeys))
	}
	for i, h := range expectedHeaders {
		if desc.HeaderKeys[i] != h {
			t.Errorf("At index %d: expected header key %q, got %q", i, h, desc.HeaderKeys[i])
		}
	}

	req := &spb.OpenMaterializedViewRequest{
		MaterializedViewName: "projects/p/instances/i/materializedViews/mv",
		AppProfileId:         "app-profile-3",
		Permission:           spb.OpenMaterializedViewRequest_PERMISSION_READ,
	}

	// Test LogNameFn
	logName := desc.LogNameFn(req)
	expectedLogName := "MaterializedViewSession(view=projects/p/instances/i/materializedViews/mv, app_profile=app-profile-3, perm=PERMISSION_READ)"
	if logName != expectedLogName {
		t.Errorf("Expected log name %q, got %q", expectedLogName, logName)
	}

	// Test MetadataFn
	meta := desc.MetadataFn(req)
	expectedMeta := map[string]string{
		"open_session.payload.materialized_view_name": "projects/p/instances/i/materializedViews/mv",
		"open_session.payload.app_profile_id":         "app-profile-3",
		"open_session.payload.permission":             "PERMISSION_READ",
	}
	if len(meta) != len(expectedMeta) {
		t.Fatalf("Expected metadata length %d, got %d", len(expectedMeta), len(meta))
	}
	for k, v := range expectedMeta {
		if meta[k] != v {
			t.Errorf("Metadata key %q: expected value %q, got %q", k, v, meta[k])
		}
	}
}

func TestSessionDescriptors_SafeAssertions(t *testing.T) {
	// 1. Verify with nil request
	if name := TABLE_SESSION.LogNameFn(nil); name != "TableSession(nil)" {
		t.Errorf("Expected LogNameFn(nil) = 'TableSession(nil)', got %q", name)
	}
	if meta := TABLE_SESSION.MetadataFn(nil); meta != nil {
		t.Errorf("Expected MetadataFn(nil) = nil, got %v", meta)
	}

	// 2. Verify with typed nil pointer
	var nilTableReq *spb.OpenTableRequest
	if name := TABLE_SESSION.LogNameFn(nilTableReq); name != "TableSession(nil)" {
		t.Errorf("Expected LogNameFn(typed nil) = 'TableSession(nil)', got %q", name)
	}
	if meta := TABLE_SESSION.MetadataFn(nilTableReq); meta != nil {
		t.Errorf("Expected MetadataFn(typed nil) = nil, got %v", meta)
	}

	// 3. Verify with wrong message type
	wrongReq := &spb.OpenAuthorizedViewRequest{}
	if name := TABLE_SESSION.LogNameFn(wrongReq); name != "TableSession(nil)" {
		t.Errorf("Expected LogNameFn(wrong type) = 'TableSession(nil)', got %q", name)
	}
	if meta := TABLE_SESSION.MetadataFn(wrongReq); meta != nil {
		t.Errorf("Expected MetadataFn(wrong type) = nil, got %v", meta)
	}
}
