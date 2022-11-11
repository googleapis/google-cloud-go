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
	"testing"

	"github.com/google/go-cmp/cmp"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

type mockTableAdminClock struct {
	btapb.BigtableTableAdminClient

	createTableReq   *btapb.CreateTableRequest
	updateTableReq   *btapb.UpdateTableRequest
	createTableResp  *btapb.Table
	updateTableError error
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
	if fmt.Sprint(err) != "TableID is required" {
		t.Fatalf("UpdateTable failed: %v", err)
	}
}

func TestTableAdmin_UpdateTable_DeletionProtection_NotProvided(t *testing.T) {
	mock := &mockTableAdminClock{}
	c := setupTableClient(t, mock)
	deletionProtection := None

	// Check if the update fails when deletion protection is not provided
	err := c.UpdateTableWithDeletionProtection(context.Background(), "My-table", deletionProtection)

	if fmt.Sprint(err) != "deletion protection is required" {
		t.Fatalf("UpdateTable failed: %v", err)
	}
}

type mockAdminClock struct {
	btapb.BigtableInstanceAdminClient

	createInstanceReq       *btapb.CreateInstanceRequest
	createClusterReq        *btapb.CreateClusterRequest
	partialUpdateClusterReq *btapb.PartialUpdateClusterRequest
	getClusterResp          *btapb.Cluster
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
		InstanceId:   "myinst",
		DisplayName:  "myinst",
		InstanceType: PRODUCTION,
		ClusterId:    "mycluster",
		Zone:         "us-central1-a",
		StorageType:  SSD,
		NumNodes:     1,
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
		ClusterID:   "mycluster",
		Zone:        "us-central1-a",
		StorageType: SSD,
		NumNodes:    1,
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
