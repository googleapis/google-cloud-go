// Copyright 2022 Google LLC
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

package bigtable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	btapb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	"cloud.google.com/go/internal/optional"
	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	longrunning "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockTableAdminClock struct {
	btapb.BigtableTableAdminClient

	createTableReq   *btapb.CreateTableRequest
	updateTableReq   *btapb.UpdateTableRequest
	createTableResp  *btapb.Table
	updateTableError error

	copyBackupReq   *btapb.CopyBackupRequest
	copyBackupError error

	modColumnReq *btapb.ModifyColumnFamiliesRequest

	createAuthorizedViewReq   *btapb.CreateAuthorizedViewRequest
	createAuthorizedViewError error
	updateAuthorizedViewReq   *btapb.UpdateAuthorizedViewRequest
	updateAuthorizedViewError error
}

func (c *mockTableAdminClock) CreateTable(
	ctx context.Context, in *btapb.CreateTableRequest, opts ...grpc.CallOption,
) (*btapb.Table, error) {
	c.createTableReq = in
	return c.createTableResp, nil
}

func (c *mockTableAdminClock) UpdateTable(
	ctx context.Context, in *btapb.UpdateTableRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.updateTableReq = in
	return &longrunning.Operation{
		Done: true,
		Result: &longrunning.Operation_Response{
			Response: &anypb.Any{TypeUrl: "google.bigtable.admin.v2.Table"},
		},
	}, c.updateTableError
}

func (c *mockTableAdminClock) CopyBackup(
	ctx context.Context, in *btapb.CopyBackupRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.copyBackupReq = in
	c.copyBackupError = fmt.Errorf("Mock error from client API")
	return nil, c.copyBackupError
}

func (c *mockTableAdminClock) ModifyColumnFamilies(
	ctx context.Context, in *btapb.ModifyColumnFamiliesRequest, opts ...grpc.CallOption) (*btapb.Table, error) {
	c.modColumnReq = in
	return nil, nil
}

func (c *mockTableAdminClock) CreateAuthorizedView(
	ctx context.Context, in *btapb.CreateAuthorizedViewRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.createAuthorizedViewReq = in
	return &longrunning.Operation{
		Done: true,
		Result: &longrunning.Operation_Response{
			Response: &anypb.Any{TypeUrl: "google.bigtable.admin.v2.AuthorizedView"},
		},
	}, c.createAuthorizedViewError
}

func (c *mockTableAdminClock) UpdateAuthorizedView(
	ctx context.Context, in *btapb.UpdateAuthorizedViewRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.updateAuthorizedViewReq = in
	return &longrunning.Operation{
		Done: true,
		Result: &longrunning.Operation_Response{
			Response: &anypb.Any{TypeUrl: "google.bigtable.admin.v2.AuthorizedView"},
		},
	}, c.updateAuthorizedViewError
}

func setupTableClient(t *testing.T, ac btapb.BigtableTableAdminClient) *AdminClient {
	ctx := context.Background()
	c, err := NewAdminClient(ctx, "my-cool-project", "my-cool-instance")
	if err != nil {
		t.Fatalf("NewAdminClient failed: %v", err)
	}
	c.tClient = ac
	return c
}

func TestTableAdmin_CreateTableFromConf_DeletionProtection_Protected(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	deletionProtection := Protected
	err := c.CreateTableFromConf(context.Background(), &TableConf{TableID: "My-table", DeletionProtection: deletionProtection})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "My-table") {
		t.Errorf("Unexpected table ID: %v, expected %v", createTableReq.TableId, "My-table")
	}
	if !cmp.Equal(createTableReq.Table.DeletionProtection, true) {
		t.Errorf("Unexpected table deletion protection: %v, expected %v", createTableReq.Table.DeletionProtection, true)
	}
}

func TestTableAdmin_CreateTableFromConf_DeletionProtection_Unprotected(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	deletionProtection := Unprotected
	err := c.CreateTableFromConf(context.Background(), &TableConf{TableID: "My-table", DeletionProtection: deletionProtection})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "My-table") {
		t.Errorf("Unexpected table ID: %v, expected %v", createTableReq.TableId, "My-table")
	}
	if !cmp.Equal(createTableReq.Table.DeletionProtection, false) {
		t.Errorf("Unexpected table deletion protection: %v, expected %v", createTableReq.Table.DeletionProtection, false)
	}
}

func TestTableAdmin_CreateTableFromConf_ChangeStream_Valid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	changeStreamRetention, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("ChangeStreamRetention not valid: %v", err)
	}
	err = c.CreateTableFromConf(context.Background(), &TableConf{TableID: "My-table", ChangeStreamRetention: changeStreamRetention})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "My-table") {
		t.Errorf("Unexpected table ID: %v, expected %v", createTableReq.TableId, "My-table")
	}
	if !cmp.Equal(createTableReq.Table.ChangeStreamConfig.RetentionPeriod.Seconds, int64(changeStreamRetention.Seconds())) {
		t.Errorf("Unexpected table change stream retention: %v, expected %v", createTableReq.Table.ChangeStreamConfig.RetentionPeriod.Seconds, changeStreamRetention.Seconds())
	}
}

func TestTableAdmin_CreateTableFromConf_AutomatedBackupPolicy_Valid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	retentionPeriod, err := time.ParseDuration("72h")
	if err != nil {
		t.Fatalf("RetentionPeriod not valid: %v", err)
	}
	frequency, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("Frequency not valid: %v", err)
	}
	automatedBackupPolicy := TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod, Frequency: frequency}

	err = c.CreateTableFromConf(context.Background(), &TableConf{TableID: "My-table", AutomatedBackupConfig: &automatedBackupPolicy})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "My-table") {
		t.Errorf("Unexpected table ID: %v, expected %v", createTableReq.TableId, "My-table")
	}
	if !cmp.Equal(createTableReq.Table.GetAutomatedBackupPolicy().Frequency.Seconds, int64(automatedBackupPolicy.Frequency.(time.Duration).Seconds())) {
		t.Errorf("Unexpected table automated backup policy frequency: %v, expected %v", createTableReq.Table.GetAutomatedBackupPolicy().Frequency.Seconds, automatedBackupPolicy.Frequency.(time.Duration))
	}
	if !cmp.Equal(createTableReq.Table.GetAutomatedBackupPolicy().RetentionPeriod.Seconds, int64(automatedBackupPolicy.RetentionPeriod.(time.Duration).Seconds())) {
		t.Errorf("Unexpected table automated backup policy retention period: %v, expected %v", createTableReq.Table.GetAutomatedBackupPolicy().Frequency.Seconds, automatedBackupPolicy.Frequency.(time.Duration))
	}
}

func TestTableAdmin_CreateTableFromConf_WithRowKeySchema(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.CreateTableFromConf(context.Background(), &TableConf{TableID: "my-table", RowKeySchema: &StructType{
		Fields: []StructField{
			{FieldName: "key1", FieldType: Int64Type{Encoding: Int64OrderedCodeBytesEncoding{}}},
			{FieldName: "key2", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}},
		},
		Encoding: StructDelimitedBytesEncoding{Delimiter: []byte{'#', '#'}},
	}})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "my-table") {
		t.Errorf("Unexpected tableID: %v, want: %v", createTableReq.TableId, "my-table")
	}
	if createTableReq.Table.RowKeySchema == nil {
		t.Errorf("Unexpected nil RowKeySchema in request")
	}
	if len(createTableReq.Table.RowKeySchema.Fields) != 2 {
		t.Errorf("Unexpected field length in row key schema: %v, want: %v", createTableReq.Table.RowKeySchema, 2)
	}
}

func TestTableAdmin_CreateBackupWithOptions_NoExpiryTime(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.CreateBackupWithOptions(context.Background(), "table", "cluster", "backup-01")
	if err == nil || !errors.Is(err, errExpiryMissing) {
		t.Errorf("CreateBackupWithOptions got: %v, want: %v error", err, errExpiryMissing)
	}
}

func TestTableAdmin_CopyBackup_ErrorFromClient(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	currTime := time.Now()
	err := c.CopyBackup(context.Background(), "source-cluster", "source-backup", "dest-project", "dest-instance", "dest-cluster", "dest-backup", currTime)
	if err == nil {
		t.Errorf("CopyBackup got: nil, want: non-nil error")
	}

	got := mock.copyBackupReq
	want := &btapb.CopyBackupRequest{
		Parent:       "projects/dest-project/instances/dest-instance/clusters/dest-cluster",
		BackupId:     "dest-backup",
		SourceBackup: "projects/my-cool-project/instances/my-cool-instance/clusters/source-cluster/backups/source-backup",
		ExpireTime:   timestamppb.New(currTime),
	}
	if diff := testutil.Diff(got, want, cmp.AllowUnexported(btapb.CopyBackupRequest{})); diff != "" {
		t.Errorf("CopyBackupRequest \ngot:\n%v,\nwant:\n%v,\ndiff:\n%v", pretty.Value(got), pretty.Value(want), diff)
	}
}

func TestTableAdmin_CreateTableFromConf_ChangeStream_Disable(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	changeStreamRetention := time.Duration(0)
	err := c.CreateTableFromConf(context.Background(), &TableConf{TableID: "My-table", ChangeStreamRetention: changeStreamRetention})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "My-table") {
		t.Errorf("Unexpected table ID: %v, expected %v", createTableReq.TableId, "My-table")
	}
	if createTableReq.Table.ChangeStreamConfig != nil {
		t.Errorf("Unexpected table change stream retention: %v should be empty", createTableReq.Table.ChangeStreamConfig)
	}
}

func TestTableAdmin_CreateTableFromConf_AutomatedBackupPolicy_Disable(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.CreateTableFromConf(context.Background(), &TableConf{TableID: "My-table", AutomatedBackupConfig: nil})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}
	createTableReq := mock.createTableReq
	if !cmp.Equal(createTableReq.TableId, "My-table") {
		t.Errorf("Unexpected table ID: %v, expected %v", createTableReq.TableId, "My-table")
	}
	if createTableReq.Table.AutomatedBackupConfig != nil {
		t.Errorf("Unexpected table automated backup policy %v should be empty", createTableReq.Table.AutomatedBackupConfig)
	}
	if createTableReq.Table.GetAutomatedBackupPolicy() != nil {
		t.Errorf("Unexpected table automated backup policy %v should be empty", createTableReq.Table.GetAutomatedBackupPolicy())
	}
}

func TestTableAdmin_UpdateTableWithDeletionProtection(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	deletionProtection := Protected

	// Check if the deletion protection updates correctly
	err := c.UpdateTableWithDeletionProtection(context.Background(), "My-table", deletionProtection)
	if err != nil {
		t.Fatalf("UpdateTableWithDeletionProtection failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if !cmp.Equal(updateTableReq.Table.DeletionProtection, true) {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "deletion_protection") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
}

func TestTableAdmin_UpdateTable_WithError(t *testing.T) {
	mock := &mockTableAdminClock{updateTableError: errors.New("update table failure error")}
	c := setupTableClient(t, mock)
	deletionProtection := Protected

	// Check if the update fails when update table returns an error
	err := c.UpdateTableWithDeletionProtection(context.Background(), "My-table", deletionProtection)

	if fmt.Sprint(err) != "error from update: update table failure error" {
		t.Fatalf("UpdateTable updated by mistake: %v", err)
	}
}

func TestTableAdmin_UpdateTable_TableID_NotProvided(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	deletionProtection := Protected

	// Check if the update fails when TableID is not provided
	err := c.UpdateTableWithDeletionProtection(context.Background(), "", deletionProtection)
	if !strings.Contains(fmt.Sprint(err), "TableID is required") {
		t.Fatalf("UpdateTable failed: %v", err)
	}
}

func TestTableAdmin_UpdateTableWithChangeStreamRetention(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	changeStreamRetention, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("ChangeStreamRetention not valid: %v", err)
	}

	err = c.UpdateTableWithChangeStream(context.Background(), "My-table", changeStreamRetention)
	if err != nil {
		t.Fatalf("UpdateTableWithChangeStream failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if !cmp.Equal(updateTableReq.Table.ChangeStreamConfig.RetentionPeriod.Seconds, int64(changeStreamRetention.Seconds())) {
		t.Errorf("UpdateTableRequest does not match, ChangeStreamConfig: %v", updateTableReq.Table.ChangeStreamConfig)
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "change_stream_config.retention_period") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[0])
	}
}

func TestTableAdmin_UpdateTableDisableChangeStream(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.UpdateTableDisableChangeStream(context.Background(), "My-table")
	if err != nil {
		t.Fatalf("UpdateTableDisableChangeStream failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if updateTableReq.Table.ChangeStreamConfig != nil {
		t.Errorf("UpdateTableRequest does not match, ChangeStreamConfig: %v should be empty", updateTableReq.Table.ChangeStreamConfig)
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "change_stream_config") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[0])
	}
}

func TestTableAdmin_UpdateTableWithRowKeySchema(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	rks := StructType{
		Fields: []StructField{
			{FieldName: "key1", FieldType: Int64Type{Encoding: Int64OrderedCodeBytesEncoding{}}},
			{FieldName: "key2", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}},
		},
		Encoding: StructDelimitedBytesEncoding{Delimiter: []byte{'#', '#'}}}
	err := c.UpdateTableWithRowKeySchema(context.Background(), "my-table", rks)
	if err != nil {
		t.Fatalf("UpdateTableWithRowKeySchema error: %v", err)
	}
	req := mock.updateTableReq

	expectedTableName := "projects/my-cool-project/instances/my-cool-instance/tables/my-table"
	if !cmp.Equal(req.Table.Name, expectedTableName) {
		t.Errorf("Unexpected table name: %v, want: %v", req.Table.Name, expectedTableName)
	}
	if !cmp.Equal(len(req.UpdateMask.Paths), 1) {
		t.Errorf("Unexpected mask size: %v, want: %v", len(req.UpdateMask.Paths), 1)
	}
	if !cmp.Equal(req.UpdateMask.Paths[0], "row_key_schema") {
		t.Errorf("Unexpected mask path: %v, want: %v", req.UpdateMask.Paths[0], "row_key_schema")
	}
}

func TestTableAdmin_UpdateTableWithClearRowKeySchema(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	err := c.UpdateTableRemoveRowKeySchema(context.Background(), "my-table")
	if err != nil {
		t.Fatalf("UpdateTableWithRowKeySchema error: %v", err)
	}
	req := mock.updateTableReq
	expectedTableName := "projects/my-cool-project/instances/my-cool-instance/tables/my-table"
	if !cmp.Equal(req.Table.Name, expectedTableName) {
		t.Errorf("Unexpected table name: %v, want: %v", req.Table.Name, expectedTableName)
	}
	if !cmp.Equal(len(req.UpdateMask.Paths), 1) {
		t.Errorf("Unexpected mask size: %v, want: %v", len(req.UpdateMask.Paths), 1)
	}
	if !cmp.Equal(req.UpdateMask.Paths[0], "row_key_schema") {
		t.Errorf("Unexpected mask path: %v, want: %v", req.UpdateMask.Paths[0], "row_key_schema")
	}
	if req.Table.RowKeySchema != nil {
		t.Errorf("Unexpected row key schema in table during clear schema request: %v", req.Table.RowKeySchema)
	}
	if !req.IgnoreWarnings {
		t.Errorf("Expect IgnoreWarnings set to true when clearing row key schema")
	}
}

func TestTableAdmin_SetGcPolicy(t *testing.T) {
	for _, test := range []struct {
		desc string
		opts UpdateFamilyOption
		want bool
	}{
		{
			desc: "IgnoreWarnings: false",
			want: false,
		},
		{
			desc: "IgnoreWarnings: true",
			opts: IgnoreWarnings(),
			want: true,
		},
	} {

		mock := &mockTableAdminClock{}
		c := setupTableClient(t, mock)

		err := c.SetGCPolicyWithOptions(context.Background(), "My-table", "cf1", NoGcPolicy(), test.opts)
		if err != nil {
			t.Fatalf("%v: Failed to set GC Policy: %v", test.desc, err)
		}

		modColumnReq := mock.modColumnReq
		if modColumnReq.IgnoreWarnings != test.want {
			t.Errorf("%v: IgnoreWarnings got: %v, want: %v", test.desc, modColumnReq.IgnoreWarnings, test.want)
		}
	}

	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.SetGCPolicy(context.Background(), "My-table", "cf1", NoGcPolicy())
	if err != nil {
		t.Fatalf("SetGCPolicy: Failed to set GC Policy: %v", err)
	}

	modColumnReq := mock.modColumnReq
	if modColumnReq.IgnoreWarnings {
		t.Errorf("SetGCPolicy: IgnoreWarnings should be set to false")
	}
}

func TestTableAdmin_CreateAuthorizedView_DeletionProtection_Protected(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.CreateTableFromConf(context.Background(), &TableConf{TableID: "my-cool-table"})
	if err != nil {
		t.Fatalf("CreateTableFromConf failed: %v", err)
	}

	deletionProtection := Protected
	err = c.CreateAuthorizedView(context.Background(), &AuthorizedViewConf{
		TableID:            "my-cool-table",
		AuthorizedViewID:   "my-cool-authorized-view",
		AuthorizedView:     &SubsetViewConf{},
		DeletionProtection: deletionProtection,
	})
	if err != nil {
		t.Fatalf("CreateAuthorizedView failed: %v", err)
	}
	createAuthorizedViewReq := mock.createAuthorizedViewReq
	if !cmp.Equal(createAuthorizedViewReq.Parent, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table") {
		t.Errorf("Unexpected parent: %v, expected %v", createAuthorizedViewReq.Parent, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table")
	}
	if !cmp.Equal(createAuthorizedViewReq.AuthorizedViewId, "my-cool-authorized-view") {
		t.Errorf("Unexpected authorized view ID: %v, expected %v", createAuthorizedViewReq.Parent, "my-cool-authorized-view")
	}
	if !cmp.Equal(createAuthorizedViewReq.AuthorizedView.DeletionProtection, true) {
		t.Errorf("Unexpected authorized view deletion protection: %v, expected %v", createAuthorizedViewReq.AuthorizedView.DeletionProtection, true)
	}
}

func TestTableAdmin_CreateAuthorizedView_DeletionProtection_Unprotected(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	deletionProtection := Unprotected
	err := c.CreateAuthorizedView(context.Background(), &AuthorizedViewConf{
		TableID:            "my-cool-table",
		AuthorizedViewID:   "my-cool-authorized-view",
		AuthorizedView:     &SubsetViewConf{},
		DeletionProtection: deletionProtection,
	})
	if err != nil {
		t.Fatalf("CreateAuthorizedView failed: %v", err)
	}
	createAuthorizedViewReq := mock.createAuthorizedViewReq
	if !cmp.Equal(createAuthorizedViewReq.Parent, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table") {
		t.Errorf("Unexpected parent: %v, expected %v", createAuthorizedViewReq.Parent, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table")
	}
	if !cmp.Equal(createAuthorizedViewReq.AuthorizedViewId, "my-cool-authorized-view") {
		t.Errorf("Unexpected authorized view ID: %v, expected %v", createAuthorizedViewReq.Parent, "my-cool-authorized-view")
	}
	if !cmp.Equal(createAuthorizedViewReq.AuthorizedView.DeletionProtection, false) {
		t.Errorf("Unexpected authorized view deletion protection: %v, expected %v", createAuthorizedViewReq.AuthorizedView.DeletionProtection, false)
	}
}

func TestTableAdmin_UpdateAuthorizedViewWithDeletionProtection(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	deletionProtection := Protected

	// Check if the deletion protection updates correctly
	err := c.UpdateAuthorizedView(context.Background(), UpdateAuthorizedViewConf{
		AuthorizedViewConf: AuthorizedViewConf{
			TableID:            "my-cool-table",
			AuthorizedViewID:   "my-cool-authorized-view",
			DeletionProtection: deletionProtection,
		},
	})
	if err != nil {
		t.Fatalf("UpdateAuthorizedView failed: %v", err)
	}
	updateAuthorizedViewReq := mock.updateAuthorizedViewReq
	if !cmp.Equal(updateAuthorizedViewReq.AuthorizedView.Name, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table/authorizedViews/my-cool-authorized-view") {
		t.Errorf("UpdateAuthorizedViewRequest does not match: AuthorizedViewName: %v, expected %v", updateAuthorizedViewReq.AuthorizedView.Name, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table/authorizedViews/my-cool-authorized-view")
	}
	if !cmp.Equal(updateAuthorizedViewReq.AuthorizedView.DeletionProtection, true) {
		t.Errorf("UpdateAuthorizedViewRequest does not match: DeletionProtection: %v, expected %v", updateAuthorizedViewReq.AuthorizedView.DeletionProtection, true)
	}
	if !cmp.Equal(len(updateAuthorizedViewReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateAuthorizedViewRequest does not match: UpdateMask has length of %d, expected %v", len(updateAuthorizedViewReq.UpdateMask.Paths), 1)
	}
	if !cmp.Equal(updateAuthorizedViewReq.UpdateMask.Paths[0], "deletion_protection") {
		t.Errorf("UpdateAuthorizedViewRequest does not match: updateAuthorizedViewReq.UpdateMask.Paths[0]: %v, expected: %v", updateAuthorizedViewReq.UpdateMask.Paths[0], "deletion_protection")
	}
}

func TestTableAdmin_UpdateAuthorizedViewWithSubsetView(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.UpdateAuthorizedView(context.Background(), UpdateAuthorizedViewConf{
		AuthorizedViewConf: AuthorizedViewConf{
			TableID:          "my-cool-table",
			AuthorizedViewID: "my-cool-authorized-view",
			AuthorizedView:   &SubsetViewConf{},
		},
	})
	if err != nil {
		t.Fatalf("UpdateAuthorizedView failed: %v", err)
	}
	updateAuthorizedViewReq := mock.updateAuthorizedViewReq
	if !cmp.Equal(updateAuthorizedViewReq.AuthorizedView.Name, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table/authorizedViews/my-cool-authorized-view") {
		t.Errorf("UpdateAuthorizedViewRequest does not match: AuthorizedViewName: %v, expected %v", updateAuthorizedViewReq.AuthorizedView.Name, "projects/my-cool-project/instances/my-cool-instance/tables/my-cool-table/authorizedViews/my-cool-authorized-view")
	}
	if !cmp.Equal(len(updateAuthorizedViewReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateAuthorizedViewRequest does not match: UpdateMask has length of %d, expected %v", len(updateAuthorizedViewReq.UpdateMask.Paths), 1)
	}
	if !cmp.Equal(updateAuthorizedViewReq.UpdateMask.Paths[0], "subset_view") {
		t.Errorf("UpdateAuthorizedViewRequest does not match: updateAuthorizedViewReq.UpdateMask.Paths[0]: %v, expected: %v", updateAuthorizedViewReq.UpdateMask.Paths[0], "subset_view")
	}
}

func TestTableAdmin_UpdateTableWithAutomatedBackupPolicy_Valid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	retentionPeriod, err := time.ParseDuration("72h")
	if err != nil {
		t.Fatalf("RetentionPeriod not valid: %v", err)
	}
	frequency, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("Frequency not valid: %v", err)
	}
	automatedBackupPolicy := TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod, Frequency: frequency}

	err = c.UpdateTableWithAutomatedBackupPolicy(context.Background(), "My-table", automatedBackupPolicy)
	if err != nil {
		t.Fatalf("UpdateTableWithChangeStream failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if !cmp.Equal(updateTableReq.Table.GetAutomatedBackupPolicy().RetentionPeriod.Seconds, int64(automatedBackupPolicy.RetentionPeriod.(time.Duration).Seconds())) {
		t.Errorf("UpdateTableRequest does not match, AutomatedBackupPolicy.RetentionPeriod: %v", updateTableReq.Table.GetAutomatedBackupPolicy().RetentionPeriod)
	}
	if !cmp.Equal(updateTableReq.Table.GetAutomatedBackupPolicy().Frequency.Seconds, int64(automatedBackupPolicy.Frequency.(time.Duration).Seconds())) {
		t.Errorf("UpdateTableRequest does not match, AutomatedBackupPolicy.Frequency: %v", updateTableReq.Table.GetAutomatedBackupPolicy().Frequency)
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 2) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "automated_backup_policy.retention_period") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[0])
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[1], "automated_backup_policy.frequency") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[1])
	}
}

func TestTableAdmin_UpdateTableWithAutomatedBackupPolicy_JustFrequency_Valid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	frequency, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("Frequency not valid: %v", err)
	}
	automatedBackupPolicy := TableAutomatedBackupPolicy{Frequency: frequency}

	err = c.UpdateTableWithAutomatedBackupPolicy(context.Background(), "My-table", automatedBackupPolicy)
	if err != nil {
		t.Fatalf("UpdateTableWithChangeStream failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if !cmp.Equal(updateTableReq.Table.GetAutomatedBackupPolicy().Frequency.Seconds, int64(automatedBackupPolicy.Frequency.(time.Duration).Seconds())) {
		t.Errorf("UpdateTableRequest does not match, AutomatedBackupPolicy.Frequency: %v", updateTableReq.Table.GetAutomatedBackupPolicy().Frequency)
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "automated_backup_policy.frequency") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[0])
	}
}

func TestTableAdmin_UpdateTableWithAutomatedBackupPolicy_JustRetentionPeriod_Valid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	retentionPeriod, err := time.ParseDuration("72h")
	if err != nil {
		t.Fatalf("RetentionPeriod not valid: %v", err)
	}
	automatedBackupPolicy := TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod}

	err = c.UpdateTableWithAutomatedBackupPolicy(context.Background(), "My-table", automatedBackupPolicy)
	if err != nil {
		t.Fatalf("UpdateTableWithChangeStream failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if !cmp.Equal(updateTableReq.Table.GetAutomatedBackupPolicy().RetentionPeriod.Seconds, int64(automatedBackupPolicy.RetentionPeriod.(time.Duration).Seconds())) {
		t.Errorf("UpdateTableRequest does not match, AutomatedBackupPolicy.RetentionPeriod: %v", updateTableReq.Table.GetAutomatedBackupPolicy().RetentionPeriod)
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "automated_backup_policy.retention_period") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[0])
	}
}

func TestTableAdmin_UpdateTableDisableAutomatedBackupPolicy(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.UpdateTableDisableAutomatedBackupPolicy(context.Background(), "My-table")
	if err != nil {
		t.Fatalf("UpdateTableDisableAutomatedBackupPolicy failed: %v", err)
	}
	updateTableReq := mock.updateTableReq
	if !cmp.Equal(updateTableReq.Table.Name, "projects/my-cool-project/instances/my-cool-instance/tables/My-table") {
		t.Errorf("UpdateTableRequest does not match, TableID: %v", updateTableReq.Table.Name)
	}
	if updateTableReq.Table.AutomatedBackupConfig != nil {
		t.Errorf("UpdateTableRequest does not match, AutomatedBackupConfig: %v should be empty", updateTableReq.Table.AutomatedBackupConfig)
	}
	if updateTableReq.Table.GetAutomatedBackupPolicy() != nil {
		t.Errorf("UpdateTableRequest does not match, GetAutomatedBackupPolicy: %v should be empty", updateTableReq.Table.GetAutomatedBackupPolicy())
	}
	if !cmp.Equal(len(updateTableReq.UpdateMask.Paths), 1) {
		t.Errorf("UpdateTableRequest does not match, UpdateMask has length of %d, expected 1", len(updateTableReq.UpdateMask.Paths))
	}
	if !cmp.Equal(updateTableReq.UpdateMask.Paths[0], "automated_backup_policy") {
		t.Errorf("UpdateTableRequest does not match, UpdateMask: %v", updateTableReq.UpdateMask.Paths[0])
	}
}

func TestTableAdmin_UpdateTableWithAutomatedBackupPolicy_NilFields_Invalid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.UpdateTableWithAutomatedBackupPolicy(context.Background(), "My-table", TableAutomatedBackupPolicy{nil, nil})
	if err == nil {
		t.Fatalf("Expected UpdateTableDisableAutomatedBackupPolicy to fail due to misspecified AutomatedBackupPolicy")
	}
}

func TestTableAdmin_UpdateTableWithAutomatedBackupPolicy_ZeroFields_Invalid(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)

	err := c.UpdateTableWithAutomatedBackupPolicy(context.Background(), "My-table", TableAutomatedBackupPolicy{time.Duration(0), time.Duration(0)})
	if err == nil {
		t.Fatalf("Expected UpdateTableDisableAutomatedBackupPolicy to fail due to misspecified AutomatedBackupPolicy")
	}
}

type mockAdminClock struct {
	btapb.BigtableInstanceAdminClient

	createInstanceReq       *btapb.CreateInstanceRequest
	createClusterReq        *btapb.CreateClusterRequest
	partialUpdateClusterReq *btapb.PartialUpdateClusterRequest
	getClusterResp          *btapb.Cluster
	createAppProfileReq     *btapb.CreateAppProfileRequest
	updateAppProfileReq     *btapb.UpdateAppProfileRequest
}

func (c *mockAdminClock) PartialUpdateCluster(
	ctx context.Context, in *btapb.PartialUpdateClusterRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.partialUpdateClusterReq = in
	return &longrunning.Operation{
		Done:   true,
		Result: &longrunning.Operation_Response{},
	}, nil
}

func (c *mockAdminClock) CreateInstance(
	ctx context.Context, in *btapb.CreateInstanceRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.createInstanceReq = in
	return &longrunning.Operation{
		Done: true,
		Result: &longrunning.Operation_Response{
			Response: &anypb.Any{TypeUrl: "google.bigtable.admin.v2.Instance"},
		},
	}, nil
}

func (c *mockAdminClock) CreateCluster(
	ctx context.Context, in *btapb.CreateClusterRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.createClusterReq = in
	return &longrunning.Operation{
		Done: true,
		Result: &longrunning.Operation_Response{
			Response: &anypb.Any{TypeUrl: "google.bigtable.admin.v2.Cluster"},
		},
	}, nil
}
func (c *mockAdminClock) PartialUpdateInstance(
	ctx context.Context, in *btapb.PartialUpdateInstanceRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	return &longrunning.Operation{
		Done:   true,
		Result: &longrunning.Operation_Response{},
	}, nil
}

func (c *mockAdminClock) GetCluster(
	ctx context.Context, in *btapb.GetClusterRequest, opts ...grpc.CallOption,
) (*btapb.Cluster, error) {
	return c.getClusterResp, nil
}

func (c *mockAdminClock) ListClusters(
	ctx context.Context, in *btapb.ListClustersRequest, opts ...grpc.CallOption,
) (*btapb.ListClustersResponse, error) {
	return &btapb.ListClustersResponse{Clusters: []*btapb.Cluster{c.getClusterResp}}, nil
}

func (c *mockAdminClock) CreateAppProfile(
	ctx context.Context, in *btapb.CreateAppProfileRequest, opts ...grpc.CallOption,
) (*btapb.AppProfile, error) {
	c.createAppProfileReq = in
	return in.AppProfile, nil
}

func (c *mockAdminClock) UpdateAppProfile(
	ctx context.Context, in *btapb.UpdateAppProfileRequest, opts ...grpc.CallOption,
) (*longrunning.Operation, error) {
	c.updateAppProfileReq = in
	return &longrunning.Operation{
		Done:   true,
		Result: &longrunning.Operation_Response{},
	}, nil
}

func setupClient(t *testing.T, ac btapb.BigtableInstanceAdminClient) *InstanceAdminClient {
	ctx := context.Background()
	c, err := NewInstanceAdminClient(ctx, "my-cool-project")
	if err != nil {
		t.Fatalf("NewInstanceAdminClient failed: %v", err)
	}
	c.iClient = ac
	return c
}

func TestInstanceAdmin_GetCluster(t *testing.T) {
	tcs := []struct {
		cluster    *btapb.Cluster
		wantConfig *AutoscalingConfig
		desc       string
	}{
		{
			desc: "when autoscaling is not enabled",
			cluster: &btapb.Cluster{
				Name:               ".../mycluster",
				Location:           ".../us-central1-a",
				State:              btapb.Cluster_READY,
				DefaultStorageType: btapb.StorageType_SSD,
				NodeScalingFactor:  btapb.Cluster_NODE_SCALING_FACTOR_1X,
			},
			wantConfig: nil,
		},
		{
			desc: "when autoscaling is enabled",
			cluster: &btapb.Cluster{
				Name:               ".../mycluster",
				Location:           ".../us-central1-a",
				State:              btapb.Cluster_READY,
				DefaultStorageType: btapb.StorageType_SSD,
				NodeScalingFactor:  btapb.Cluster_NODE_SCALING_FACTOR_1X,
				Config: &btapb.Cluster_ClusterConfig_{
					ClusterConfig: &btapb.Cluster_ClusterConfig{
						ClusterAutoscalingConfig: &btapb.Cluster_ClusterAutoscalingConfig{
							AutoscalingLimits: &btapb.AutoscalingLimits{
								MinServeNodes: 1,
								MaxServeNodes: 2,
							},
							AutoscalingTargets: &btapb.AutoscalingTargets{
								CpuUtilizationPercent:        10,
								StorageUtilizationGibPerNode: 3000,
							},
						},
					},
				},
			},
			wantConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			c := setupClient(t, &mockAdminClock{getClusterResp: tc.cluster})

			info, err := c.GetCluster(context.Background(), "myinst", "mycluster")
			if err != nil {
				t.Fatalf("GetCluster failed: %v", err)
			}

			if gotConfig := info.AutoscalingConfig; !cmp.Equal(gotConfig, tc.wantConfig) {
				t.Fatalf("want autoscaling config = %v, got = %v", tc.wantConfig, gotConfig)
			}
		})
	}
}

func TestInstanceAdmin_Clusters(t *testing.T) {
	tcs := []struct {
		cluster    *btapb.Cluster
		wantConfig *AutoscalingConfig
		desc       string
	}{
		{
			desc: "when autoscaling is not enabled",
			cluster: &btapb.Cluster{
				Name:               ".../mycluster",
				Location:           ".../us-central1-a",
				State:              btapb.Cluster_READY,
				DefaultStorageType: btapb.StorageType_SSD,
				NodeScalingFactor:  btapb.Cluster_NODE_SCALING_FACTOR_1X,
			},
			wantConfig: nil,
		},
		{
			desc: "when autoscaling is enabled",
			cluster: &btapb.Cluster{
				Name:               ".../mycluster",
				Location:           ".../us-central1-a",
				State:              btapb.Cluster_READY,
				DefaultStorageType: btapb.StorageType_SSD,
				NodeScalingFactor:  btapb.Cluster_NODE_SCALING_FACTOR_1X,
				Config: &btapb.Cluster_ClusterConfig_{
					ClusterConfig: &btapb.Cluster_ClusterConfig{
						ClusterAutoscalingConfig: &btapb.Cluster_ClusterAutoscalingConfig{
							AutoscalingLimits: &btapb.AutoscalingLimits{
								MinServeNodes: 1,
								MaxServeNodes: 2,
							},
							AutoscalingTargets: &btapb.AutoscalingTargets{
								CpuUtilizationPercent:        10,
								StorageUtilizationGibPerNode: 3000,
							},
						},
					},
				},
			},
			wantConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			c := setupClient(t, &mockAdminClock{getClusterResp: tc.cluster})

			infos, err := c.Clusters(context.Background(), "myinst")
			if err != nil {
				t.Fatalf("Clusters failed: %v", err)
			}
			if len(infos) != 1 {
				t.Fatalf("Clusters len: want = 1, got = %v", len(infos))
			}

			info := infos[0]
			if gotConfig := info.AutoscalingConfig; !cmp.Equal(gotConfig, tc.wantConfig) {
				t.Fatalf("want autoscaling config = %v, got = %v", tc.wantConfig, gotConfig)
			}
		})
	}
}

func TestInstanceAdmin_SetAutoscaling(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.SetAutoscaling(context.Background(), "myinst", "mycluster", AutoscalingConfig{
		MinNodes:                  1,
		MaxNodes:                  2,
		CPUTargetPercent:          10,
		StorageUtilizationPerNode: 3000,
	})
	if err != nil {
		t.Fatalf("SetAutoscaling failed: %v", err)
	}

	wantMask := []string{"cluster_config.cluster_autoscaling_config"}
	if gotMask := mock.partialUpdateClusterReq.UpdateMask.Paths; !cmp.Equal(wantMask, gotMask) {
		t.Fatalf("want update mask = %v, got = %v", wantMask, gotMask)
	}

	wantName := "projects/my-cool-project/instances/myinst/clusters/mycluster"
	if gotName := mock.partialUpdateClusterReq.Cluster.Name; gotName != wantName {
		t.Fatalf("want name = %v, got = %v", wantName, gotName)
	}

	cc := mock.partialUpdateClusterReq.Cluster.Config.(*btapb.Cluster_ClusterConfig_)
	gotConfig := cc.ClusterConfig.ClusterAutoscalingConfig

	wantMin := int32(1)
	if gotMin := gotConfig.AutoscalingLimits.MinServeNodes; wantMin != gotMin {
		t.Fatalf("want autoscaling min nodes = %v, got = %v", wantMin, gotMin)
	}

	wantMax := int32(2)
	if gotMax := gotConfig.AutoscalingLimits.MaxServeNodes; wantMax != gotMax {
		t.Fatalf("want autoscaling max nodes = %v, got = %v", wantMax, gotMax)
	}

	wantCPU := int32(10)
	if gotCPU := gotConfig.AutoscalingTargets.CpuUtilizationPercent; wantCPU != gotCPU {
		t.Fatalf("want autoscaling cpu = %v, got = %v", wantCPU, gotCPU)
	}

	wantStorage := int32(3000)
	if gotStorage := gotConfig.AutoscalingTargets.StorageUtilizationGibPerNode; wantStorage != gotStorage {
		t.Fatalf("want autoscaling storage = %v, got = %v", wantStorage, gotStorage)
	}
}

func TestInstanceAdmin_UpdateCluster_RemovingAutoscaling(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.UpdateCluster(context.Background(), "myinst", "mycluster", 1)
	if err != nil {
		t.Fatalf("UpdateCluster failed: %v", err)
	}

	wantMask := []string{"serve_nodes", "cluster_config.cluster_autoscaling_config"}
	if gotMask := mock.partialUpdateClusterReq.UpdateMask.Paths; !cmp.Equal(wantMask, gotMask) {
		t.Fatalf("want update mask = %v, got = %v", wantMask, gotMask)
	}

	if gotConfig := mock.partialUpdateClusterReq.Cluster.Config; gotConfig != nil {
		t.Fatalf("want config = nil, got = %v", gotConfig)
	}
}

func TestInstanceAdmin_CreateInstance_WithAutoscaling(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.CreateInstance(context.Background(), &InstanceConf{
		InstanceId:        "myinst",
		DisplayName:       "myinst",
		InstanceType:      PRODUCTION,
		ClusterId:         "mycluster",
		Zone:              "us-central1-a",
		StorageType:       SSD,
		NodeScalingFactor: NodeScalingFactor1X,
		AutoscalingConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
	})
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	mycc := mock.createInstanceReq.Clusters["mycluster"]
	cc := mycc.Config.(*btapb.Cluster_ClusterConfig_)
	gotConfig := cc.ClusterConfig.ClusterAutoscalingConfig

	wantMin := int32(1)
	if gotMin := gotConfig.AutoscalingLimits.MinServeNodes; wantMin != gotMin {
		t.Fatalf("want autoscaling min nodes = %v, got = %v", wantMin, gotMin)
	}

	wantMax := int32(2)
	if gotMax := gotConfig.AutoscalingLimits.MaxServeNodes; wantMax != gotMax {
		t.Fatalf("want autoscaling max nodes = %v, got = %v", wantMax, gotMax)
	}

	wantCPU := int32(10)
	if gotCPU := gotConfig.AutoscalingTargets.CpuUtilizationPercent; wantCPU != gotCPU {
		t.Fatalf("want autoscaling cpu = %v, got = %v", wantCPU, gotCPU)
	}

	err = c.CreateInstance(context.Background(), &InstanceConf{
		InstanceId:        "myinst",
		DisplayName:       "myinst",
		InstanceType:      PRODUCTION,
		ClusterId:         "mycluster",
		Zone:              "us-central1-a",
		StorageType:       SSD,
		NodeScalingFactor: NodeScalingFactor1X,
		NumNodes:          1,
	})
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// omitting autoscaling config results in a nil config in the request
	mycc = mock.createInstanceReq.Clusters["mycluster"]
	if cc := mycc.GetClusterConfig(); cc != nil {
		t.Fatalf("want config = nil, got = %v", gotConfig)
	}
}

func TestInstanceAdmin_CreateInstanceWithClusters_WithAutoscaling(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.CreateInstanceWithClusters(context.Background(), &InstanceWithClustersConfig{
		InstanceID:   "myinst",
		DisplayName:  "myinst",
		InstanceType: PRODUCTION,
		Clusters: []ClusterConfig{
			{
				ClusterID:         "mycluster",
				Zone:              "us-central1-a",
				StorageType:       SSD,
				NodeScalingFactor: NodeScalingFactor1X,
				AutoscalingConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateInstanceWithClusters failed: %v", err)
	}

	mycc := mock.createInstanceReq.Clusters["mycluster"]
	cc := mycc.Config.(*btapb.Cluster_ClusterConfig_)
	gotConfig := cc.ClusterConfig.ClusterAutoscalingConfig

	wantMin := int32(1)
	if gotMin := gotConfig.AutoscalingLimits.MinServeNodes; wantMin != gotMin {
		t.Fatalf("want autoscaling min nodes = %v, got = %v", wantMin, gotMin)
	}

	wantMax := int32(2)
	if gotMax := gotConfig.AutoscalingLimits.MaxServeNodes; wantMax != gotMax {
		t.Fatalf("want autoscaling max nodes = %v, got = %v", wantMax, gotMax)
	}

	wantCPU := int32(10)
	if gotCPU := gotConfig.AutoscalingTargets.CpuUtilizationPercent; wantCPU != gotCPU {
		t.Fatalf("want autoscaling cpu = %v, got = %v", wantCPU, gotCPU)
	}
}

func TestInstanceAdmin_CreateCluster_WithAutoscaling(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.CreateCluster(context.Background(), &ClusterConfig{
		ClusterID:         "mycluster",
		Zone:              "us-central1-a",
		StorageType:       SSD,
		AutoscalingConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
		NodeScalingFactor: NodeScalingFactor1X,
	})
	if err != nil {
		t.Fatalf("CreateCluster failed: %v", err)
	}

	cc := mock.createClusterReq.Cluster.Config.(*btapb.Cluster_ClusterConfig_)
	gotConfig := cc.ClusterConfig.ClusterAutoscalingConfig

	wantMin := int32(1)
	if gotMin := gotConfig.AutoscalingLimits.MinServeNodes; wantMin != gotMin {
		t.Fatalf("want autoscaling min nodes = %v, got = %v", wantMin, gotMin)
	}

	wantMax := int32(2)
	if gotMax := gotConfig.AutoscalingLimits.MaxServeNodes; wantMax != gotMax {
		t.Fatalf("want autoscaling max nodes = %v, got = %v", wantMax, gotMax)
	}

	wantCPU := int32(10)
	if gotCPU := gotConfig.AutoscalingTargets.CpuUtilizationPercent; wantCPU != gotCPU {
		t.Fatalf("want autoscaling cpu = %v, got = %v", wantCPU, gotCPU)
	}

	wantStorage := int32(3000)
	if gotStorage := gotConfig.AutoscalingTargets.StorageUtilizationGibPerNode; wantStorage != gotStorage {
		t.Fatalf("want autoscaling storage = %v, got = %v", wantStorage, gotStorage)
	}

	err = c.CreateCluster(context.Background(), &ClusterConfig{
		ClusterID:         "mycluster",
		Zone:              "us-central1-a",
		StorageType:       SSD,
		NumNodes:          1,
		NodeScalingFactor: NodeScalingFactor1X,
	})
	if err != nil {
		t.Fatalf("CreateCluster failed: %v", err)
	}

	// omitting autoscaling config results in a nil config in the request
	if cc := mock.createClusterReq.Cluster.GetClusterConfig(); cc != nil {
		t.Fatalf("want config = nil, got = %v", gotConfig)
	}
}

func TestInstanceAdmin_UpdateInstanceWithClusters_IgnoresInvalidClusters(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.UpdateInstanceWithClusters(context.Background(), &InstanceWithClustersConfig{
		InstanceID:  "myinst",
		DisplayName: "myinst",
		Clusters: []ClusterConfig{
			{
				ClusterID: "mycluster",
				Zone:      "us-central1-a",
				// Cluster has no autoscaling or num nodes
				// It should be ignored
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateInstanceWithClusters failed: %v", err)
	}

	if mock.partialUpdateClusterReq != nil {
		t.Fatalf("PartialUpdateCluster should not have been called, got = %v",
			mock.partialUpdateClusterReq)
	}
}

func TestInstanceAdmin_UpdateInstanceWithClusters_WithAutoscaling(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)

	err := c.UpdateInstanceWithClusters(context.Background(), &InstanceWithClustersConfig{
		InstanceID:  "myinst",
		DisplayName: "myinst",
		Clusters: []ClusterConfig{
			{
				ClusterID:         "mycluster",
				Zone:              "us-central1-a",
				AutoscalingConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateInstanceWithClusters failed: %v", err)
	}

	cc := mock.partialUpdateClusterReq.Cluster.Config.(*btapb.Cluster_ClusterConfig_)
	gotConfig := cc.ClusterConfig.ClusterAutoscalingConfig

	wantMin := int32(1)
	if gotMin := gotConfig.AutoscalingLimits.MinServeNodes; wantMin != gotMin {
		t.Fatalf("want autoscaling min nodes = %v, got = %v", wantMin, gotMin)
	}

	wantMax := int32(2)
	if gotMax := gotConfig.AutoscalingLimits.MaxServeNodes; wantMax != gotMax {
		t.Fatalf("want autoscaling max nodes = %v, got = %v", wantMax, gotMax)
	}

	wantCPU := int32(10)
	if gotCPU := gotConfig.AutoscalingTargets.CpuUtilizationPercent; wantCPU != gotCPU {
		t.Fatalf("want autoscaling cpu = %v, got = %v", wantCPU, gotCPU)
	}

	wantStorage := int32(3000)
	if gotStorage := gotConfig.AutoscalingTargets.StorageUtilizationGibPerNode; wantStorage != gotStorage {
		t.Fatalf("want autoscaling storage = %v, got = %v", wantStorage, gotStorage)
	}

	err = c.UpdateInstanceWithClusters(context.Background(), &InstanceWithClustersConfig{
		InstanceID:  "myinst",
		DisplayName: "myinst",
		Clusters: []ClusterConfig{
			{
				ClusterID: "mycluster",
				Zone:      "us-central1-a",
				NumNodes:  1,
				// no autoscaling config
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateInstanceWithClusters failed: %v", err)
	}

	got := mock.partialUpdateClusterReq.Cluster.Config
	if got != nil {
		t.Fatalf("want autoscaling config = nil, got = %v", gotConfig)
	}
}

func TestInstanceAdmin_UpdateInstanceAndSyncClusters_WithAutoscaling(t *testing.T) {
	mock := &mockAdminClock{
		getClusterResp: &btapb.Cluster{
			Name:               ".../mycluster",
			Location:           ".../us-central1-a",
			State:              btapb.Cluster_READY,
			DefaultStorageType: btapb.StorageType_SSD,
			Config: &btapb.Cluster_ClusterConfig_{
				ClusterConfig: &btapb.Cluster_ClusterConfig{
					ClusterAutoscalingConfig: &btapb.Cluster_ClusterAutoscalingConfig{
						AutoscalingLimits: &btapb.AutoscalingLimits{
							MinServeNodes: 1,
							MaxServeNodes: 2,
						},
						AutoscalingTargets: &btapb.AutoscalingTargets{
							CpuUtilizationPercent:        10,
							StorageUtilizationGibPerNode: 3000,
						},
					},
				},
			},
		},
	}
	c := setupClient(t, mock)

	_, err := UpdateInstanceAndSyncClusters(context.Background(), c, &InstanceWithClustersConfig{
		InstanceID:  "myinst",
		DisplayName: "myinst",
		Clusters: []ClusterConfig{
			{
				ClusterID:         "mycluster",
				Zone:              "us-central1-a",
				AutoscalingConfig: &AutoscalingConfig{MinNodes: 1, MaxNodes: 2, CPUTargetPercent: 10, StorageUtilizationPerNode: 3000},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateInstanceAndSyncClusters failed: %v", err)
	}

	cc := mock.partialUpdateClusterReq.Cluster.Config.(*btapb.Cluster_ClusterConfig_)
	gotConfig := cc.ClusterConfig.ClusterAutoscalingConfig

	wantMin := int32(1)
	if gotMin := gotConfig.AutoscalingLimits.MinServeNodes; wantMin != gotMin {
		t.Fatalf("want autoscaling min nodes = %v, got = %v", wantMin, gotMin)
	}

	wantMax := int32(2)
	if gotMax := gotConfig.AutoscalingLimits.MaxServeNodes; wantMax != gotMax {
		t.Fatalf("want autoscaling max nodes = %v, got = %v", wantMax, gotMax)
	}

	wantCPU := int32(10)
	if gotCPU := gotConfig.AutoscalingTargets.CpuUtilizationPercent; wantCPU != gotCPU {
		t.Fatalf("want autoscaling cpu = %v, got = %v", wantCPU, gotCPU)
	}

	wantStorage := int32(3000)
	if gotStorage := gotConfig.AutoscalingTargets.StorageUtilizationGibPerNode; wantStorage != gotStorage {
		t.Fatalf("want autoscaling storage = %v, got = %v", wantStorage, gotStorage)
	}

	_, err = UpdateInstanceAndSyncClusters(context.Background(), c, &InstanceWithClustersConfig{
		InstanceID:  "myinst",
		DisplayName: "myinst",
		Clusters: []ClusterConfig{
			{
				ClusterID: "mycluster",
				Zone:      "us-central1-a",
				NumNodes:  1,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateInstanceAndSyncClusters failed: %v", err)
	}
	got := mock.partialUpdateClusterReq.Cluster.Config
	if got != nil {
		t.Fatalf("want autoscaling config = nil, got = %v", gotConfig)
	}
}

func TestInstanceAdmin_CreateAppProfile(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)
	ctx := context.Background()

	tests := []struct {
		desc        string
		conf        ProfileConf
		wantProfile *btapb.AppProfile
		wantErrMsg  string
	}{
		{
			desc: "SingleClusterRouting config and Deprecated SingleClusterRouting policy",
			conf: ProfileConf{
				ProfileID:   "prof1",
				InstanceID:  "inst1",
				Description: "desc1",
				RoutingConfig: &SingleClusterRoutingConfig{
					ClusterID:                "cluster2",
					AllowTransactionalWrites: false,
				},
				RoutingPolicy:            SingleClusterRouting,
				ClusterID:                "cluster1",
				AllowTransactionalWrites: true,
				Isolation:                &StandardIsolation{Priority: AppProfilePriorityHigh},
			},
			wantProfile: &btapb.AppProfile{
				Description: "desc1",
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId:                "cluster2",
						AllowTransactionalWrites: false,
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},
		{
			desc: "MultiClusterRoutingUseAny config with affinity and Deprecated SingleClusterRouting policy",
			conf: ProfileConf{
				ProfileID:                "prof3",
				InstanceID:               "inst1",
				Description:              "desc3",
				RoutingPolicy:            SingleClusterRouting,
				ClusterID:                "cluster1",
				AllowTransactionalWrites: true,
				RoutingConfig: &MultiClusterRoutingUseAnyConfig{
					ClusterIDs: []string{"cluster1"},
					Affinity:   &RowAffinity{},
				},
			},
			wantProfile: &btapb.AppProfile{
				Description: "desc3",
				RoutingPolicy: &btapb.AppProfile_MultiClusterRoutingUseAny_{
					MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{
						ClusterIds: []string{"cluster1"},
						Affinity:   &btapb.AppProfile_MultiClusterRoutingUseAny_RowAffinity_{},
					},
				},
			},
		},
		{
			desc: "Error - No routing policy",
			conf: ProfileConf{
				ProfileID:  "prof_err",
				InstanceID: "inst1",
			},
			wantErrMsg: "bigtable: invalid RoutingPolicy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mock.createAppProfileReq = nil // Reset mock request
			_, gotErr := c.CreateAppProfile(ctx, tt.conf)
			fmt.Println(gotErr)

			if !((gotErr == nil && tt.wantErrMsg == "") ||
				(gotErr != nil && tt.wantErrMsg != "" && strings.Contains(gotErr.Error(), tt.wantErrMsg))) {
				t.Fatalf("CreateAppProfile error got: %v, want: %v", gotErr, tt.wantErrMsg)
			}

			if tt.wantErrMsg != "" {
				return
			}

			// Verify the request sent to the mock
			wantReq := &btapb.CreateAppProfileRequest{
				Parent:         "projects/my-cool-project/instances/inst1",
				AppProfileId:   tt.conf.ProfileID,
				AppProfile:     tt.wantProfile,
				IgnoreWarnings: tt.conf.IgnoreWarnings,
			}
			// The name field is set by the server, so we don't compare it in the request's AppProfile
			mock.createAppProfileReq.AppProfile.Name = ""
			if diff := testutil.Diff(mock.createAppProfileReq, wantReq); diff != "" {
				t.Errorf("CreateAppProfileRequest mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestInstanceAdmin_UpdateAppProfile(t *testing.T) {
	mock := &mockAdminClock{}
	c := setupClient(t, mock)
	ctx := context.Background()

	tests := []struct {
		desc        string
		profileID   string
		instanceID  string
		updateAttrs ProfileAttrsToUpdate
		wantReq     *btapb.UpdateAppProfileRequest
		wantErr     bool
	}{
		{
			desc:       "Update only routing to SingleClusterRoutingConfig",
			profileID:  "prof2",
			instanceID: "inst1",
			updateAttrs: ProfileAttrsToUpdate{
				RoutingConfig: &SingleClusterRoutingConfig{
					ClusterID:                "cluster-new",
					AllowTransactionalWrites: false,
				},
			},
			wantReq: &btapb.UpdateAppProfileRequest{
				AppProfile: &btapb.AppProfile{
					Name: "projects/my-cool-project/instances/inst1/appProfiles/prof2",
					RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
						SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
							ClusterId:                "cluster-new",
							AllowTransactionalWrites: false,
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"single_cluster_routing"}},
			},
		},
		{
			desc:       "Update isolation to DataBoost",
			profileID:  "prof4",
			instanceID: "inst1",
			updateAttrs: ProfileAttrsToUpdate{
				Isolation: &DataBoostIsolationReadOnly{ComputeBillingOwner: HostPays},
			},
			wantReq: &btapb.UpdateAppProfileRequest{
				AppProfile: &btapb.AppProfile{
					Name: "projects/my-cool-project/instances/inst1/appProfiles/prof4",
					Isolation: &btapb.AppProfile_DataBoostIsolationReadOnly_{
						DataBoostIsolationReadOnly: &btapb.AppProfile_DataBoostIsolationReadOnly{
							ComputeBillingOwner: ptr(btapb.AppProfile_DataBoostIsolationReadOnly_HOST_PAYS),
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"data_boost_isolation_read_only"}},
			},
		},
		{
			desc:       "Update isolation to DataBoost without ComputeBillingOwner",
			profileID:  "prof4",
			instanceID: "inst1",
			updateAttrs: ProfileAttrsToUpdate{
				Isolation: &DataBoostIsolationReadOnly{},
			},
			wantReq: &btapb.UpdateAppProfileRequest{
				AppProfile: &btapb.AppProfile{
					Name: "projects/my-cool-project/instances/inst1/appProfiles/prof4",
					Isolation: &btapb.AppProfile_DataBoostIsolationReadOnly_{
						DataBoostIsolationReadOnly: &btapb.AppProfile_DataBoostIsolationReadOnly{
							ComputeBillingOwner: ptr(btapb.AppProfile_DataBoostIsolationReadOnly_COMPUTE_BILLING_OWNER_UNSPECIFIED),
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"data_boost_isolation_read_only"}},
			},
		},
		{
			desc:       "Update using both RoutingPolicy and MultiClusterRoutingUseAnyConfig",
			profileID:  "prof_dep",
			instanceID: "inst1",
			updateAttrs: ProfileAttrsToUpdate{
				RoutingPolicy: optional.String(SingleClusterRouting),
				RoutingConfig: &MultiClusterRoutingUseAnyConfig{
					ClusterIDs: []string{"c1", "c2"},
					Affinity:   &RowAffinity{},
				},
			},
			wantReq: &btapb.UpdateAppProfileRequest{
				AppProfile: &btapb.AppProfile{
					Name: "projects/my-cool-project/instances/inst1/appProfiles/prof_dep",
					RoutingPolicy: &btapb.AppProfile_MultiClusterRoutingUseAny_{
						MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{
							ClusterIds: []string{"c1", "c2"},
							Affinity:   &btapb.AppProfile_MultiClusterRoutingUseAny_RowAffinity_{},
						},
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"multi_cluster_routing_use_any"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			mock.updateAppProfileReq = nil // Reset mock request
			err := c.UpdateAppProfile(ctx, tt.instanceID, tt.profileID, tt.updateAttrs)

			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateAppProfile error got: %v, want: %v", err, tt.wantErr)
			}

			// Verify the request sent to the mock
			if diff := testutil.Diff(mock.updateAppProfileReq, tt.wantReq); diff != "" {
				t.Errorf("UpdateAppProfileRequest mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
