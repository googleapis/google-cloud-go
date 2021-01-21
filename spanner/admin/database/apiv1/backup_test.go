/*
Copyright 2020 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	longrunningpb "google.golang.org/genproto/googleapis/longrunning"
	status "google.golang.org/genproto/googleapis/rpc/status"
	databasepb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	gstatus "google.golang.org/grpc/status"
)

// CreateBackup is an extension to mockDatabaseAdminServer for managing backups.
func (s *mockDatabaseAdminServer) CreateBackup(ctx context.Context, req *databasepb.CreateBackupRequest) (*longrunningpb.Operation, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if xg := md["x-goog-api-client"]; len(xg) == 0 || !strings.Contains(xg[0], "gl-go/") {
		return nil, fmt.Errorf("x-goog-api-client = %v, expected gl-go key", xg)
	}
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*longrunningpb.Operation), nil
}

func TestDatabaseAdminClient_StartBackupOperation(t *testing.T) {
	backupID := "some-backup"
	instanceName := "projects/some-project/instances/some-instance"
	databasePath := instanceName + "/databases/some-database"
	backupPath := instanceName + "/backups/" + backupID
	expectedRequest := &databasepb.CreateBackupRequest{
		Parent:   instanceName,
		BackupId: backupID,
		Backup: &databasepb.Backup{
			Database: databasePath,
			ExpireTime: &timestamp.Timestamp{
				Seconds: 221688000,
				Nanos:   500,
			},
		},
	}
	expectedResponse := &databasepb.Backup{
		Name:      backupPath,
		Database:  databasePath,
		SizeBytes: 1796325715123,
	}
	mockDatabaseAdmin.err = nil
	mockDatabaseAdmin.reqs = nil

	ctx := context.Background()
	any, err := ptypes.MarshalAny(expectedResponse)
	if err != nil {
		t.Fatal(err)
	}
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps[:0], &longrunningpb.Operation{
		Name:   "longrunning-test",
		Done:   true,
		Result: &longrunningpb.Operation_Response{Response: any},
	})
	c, err := NewDatabaseAdminClient(ctx, clientOpt)
	if err != nil {
		t.Fatal(err)
	}
	respLRO, err := c.StartBackupOperation(ctx, backupID, databasePath, time.Unix(221688000, 500))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := respLRO.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want, got := expectedRequest, mockDatabaseAdmin.reqs[0]; !proto.Equal(want, got) {
		t.Errorf("got request %q, want %q", got, want)
	}
	if want, got := expectedResponse, resp; !proto.Equal(want, got) {
		t.Errorf("got response %q, want %q)", got, want)
	}
}

func TestDatabaseAdminStartBackupOperationError(t *testing.T) {
	wantErr := codes.PermissionDenied
	mockDatabaseAdmin.err = nil
	mockDatabaseAdmin.resps = append(mockDatabaseAdmin.resps[:0], &longrunningpb.Operation{
		Name: "longrunning-test",
		Done: true,
		Result: &longrunningpb.Operation_Error{
			Error: &status.Status{
				Code:    int32(wantErr),
				Message: "test error",
			},
		},
	})
	// Minimum expiry time is 6 hours
	expires := time.Now().Add(time.Hour * 7)
	ctx := context.Background()
	c, err := NewDatabaseAdminClient(ctx, clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	respLRO, err := c.StartBackupOperation(
		ctx,
		"some-backup",
		"projects/some-project/instances/some-instance/databases/some-database",
		expires,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, reqerr := respLRO.Wait(ctx)
	st, ok := gstatus.FromError(reqerr)
	if !ok {
		t.Fatalf("got error %v, expected grpc error", reqerr)
	}
	if st.Code() != wantErr {
		t.Fatalf("got error code %q, want %q", st.Code(), wantErr)
	}
}
