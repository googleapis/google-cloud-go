// Copyright 2026 Google LLC
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

package admin

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	adminpb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type testEnv struct {
	client   *BigtableTableAdminClient
	project  string
	instance string
	cluster  string
}

var (
	sourceTableSpace   = uid.NewSpace("it-src-table", &uid.Options{Short: true})
	backupSpace        = uid.NewSpace("it-backup", &uid.Options{Short: true})
	restoredTableSpace = uid.NewSpace("it-restored-table", &uid.Options{Short: true})
	replTestTableSpace = uid.NewSpace("it-repl-table", &uid.Options{Short: true})
)

func setupIntegration(t *testing.T) *testEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	emulatorHost := os.Getenv("BIGTABLE_EMULATOR_HOST")
	var opts []option.ClientOption
	project := os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID")
	instance := os.Getenv("GCLOUD_TESTS_BIGTABLE_INSTANCE")
	cluster := os.Getenv("GCLOUD_TESTS_BIGTABLE_CLUSTER")

	if emulatorHost != "" {
		t.Logf("Using emulator at %s", emulatorHost)
		opts = append(opts,
			option.WithEndpoint(emulatorHost),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
		if project == "" {
			project = "test-project"
		}
		if instance == "" {
			instance = "test-instance"
		}
		if cluster == "" {
			cluster = "test-cluster"
		}
	} else if project == "" || instance == "" {
		t.Skip("Missing GCLOUD_TESTS_GOLANG_PROJECT_ID or GCLOUD_TESTS_BIGTABLE_INSTANCE for non-emulator run")
	}

	client, err := NewBigtableTableAdminClient(ctx, opts...)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
	})

	return &testEnv{
		client:   client,
		project:  project,
		instance: instance,
		cluster:  cluster,
	}
}

func TestIntegration_RestoreTable(t *testing.T) {
	t.Parallel()
	env := setupIntegration(t)
	if os.Getenv("BIGTABLE_EMULATOR_HOST") == "" && env.cluster == "" {
		t.Skip("Missing GCLOUD_TESTS_BIGTABLE_CLUSTER for non-emulator run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)
	cleanupCtx := context.WithoutCancel(ctx)
	client := env.client

	sourceTableID := sourceTableSpace.New()
	backupID := backupSpace.New()
	restoredTableID := restoredTableSpace.New()

	instancePath := fmt.Sprintf("projects/%s/instances/%s", env.project, env.instance)
	sourceTablePath := fmt.Sprintf("%s/tables/%s", instancePath, sourceTableID)
	clusterPath := fmt.Sprintf("%s/clusters/%s", instancePath, env.cluster)
	backupPath := fmt.Sprintf("%s/backups/%s", clusterPath, backupID)
	restoredTablePath := fmt.Sprintf("%s/tables/%s", instancePath, restoredTableID)

	// 1. Create source table
	_, err := client.CreateTable(ctx, &adminpb.CreateTableRequest{
		Parent:  instancePath,
		TableId: sourceTableID,
		Table:   &adminpb.Table{},
	})
	if err != nil {
		t.Fatalf("Failed to create source table: %v", err)
	}
	t.Cleanup(func() {
		client.DeleteTable(cleanupCtx, &adminpb.DeleteTableRequest{Name: sourceTablePath})
	})

	// 2. Create backup
	expireTime := time.Now().Add(7 * time.Hour)
	opCreateBackup, err := client.CreateBackup(ctx, &adminpb.CreateBackupRequest{
		Parent:   clusterPath,
		BackupId: backupID,
		Backup: &adminpb.Backup{
			SourceTable: sourceTablePath,
			ExpireTime:  timestamppb.New(expireTime),
		},
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unimplemented {
			t.Skip("Emulator does not support CreateBackup")
		}
		t.Fatalf("Failed to initiate backup: %v", err)
	}
	t.Cleanup(func() {
		client.DeleteBackup(cleanupCtx, &adminpb.DeleteBackupRequest{Name: backupPath})
	})

	_, err = opCreateBackup.Wait(ctx)
	if err != nil {
		t.Fatalf("Backup LRO failed: %v", err)
	}

	// 3. Restore table
	err = client.RestoreTable(ctx, &adminpb.RestoreTableRequest{
		Parent:  instancePath,
		TableId: restoredTableID,
		Source: &adminpb.RestoreTableRequest_Backup{
			Backup: backupPath,
		},
	})
	if err != nil {
		t.Fatalf("RestoreTable failed: %v", err)
	}
	t.Cleanup(func() {
		client.DeleteTable(cleanupCtx, &adminpb.DeleteTableRequest{Name: restoredTablePath})
	})

	// 4. Verify restored table exists
	restoredTable, err := client.GetTable(ctx, &adminpb.GetTableRequest{
		Name: restoredTablePath,
	})
	if err != nil {
		t.Fatalf("Failed to get restored table: %v", err)
	}

	if restoredTable.Name != restoredTablePath {
		t.Errorf("Expected restored table name %q, got %q", restoredTablePath, restoredTable.Name)
	}
}

func TestIntegration_WaitForConsistency(t *testing.T) {
	t.Parallel()
	env := setupIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	cleanupCtx := context.WithoutCancel(ctx)
	client := env.client

	tableID := replTestTableSpace.New()
	instancePath := fmt.Sprintf("projects/%s/instances/%s", env.project, env.instance)
	tablePath := fmt.Sprintf("%s/tables/%s", instancePath, tableID)

	// Create table
	_, err := client.CreateTable(ctx, &adminpb.CreateTableRequest{
		Parent:  instancePath,
		TableId: tableID,
		Table:   &adminpb.Table{},
	})
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}
	t.Cleanup(func() {
		client.DeleteTable(cleanupCtx, &adminpb.DeleteTableRequest{Name: tablePath})
	})

	// Wait for replication
	err = client.WaitForConsistency(ctx, tablePath)
	if err != nil {
		t.Fatalf("WaitForConsistency failed: %v", err)
	}
}
