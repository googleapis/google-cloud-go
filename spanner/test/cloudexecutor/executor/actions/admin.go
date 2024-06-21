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

package actions

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// AdminActionHandler holds the necessary components and options required for performing admin tasks.
type AdminActionHandler struct {
	Action        *executorpb.AdminAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
	Options       []option.ClientOption
}

// ExecuteAction execute admin actions by action case, using OutcomeSender to send status and results back.
func (h *AdminActionHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("executing admin action %v", h.Action)
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	var err error
	switch h.Action.GetAction().(type) {
	case *executorpb.AdminAction_CreateCloudInstance:
		err = executeCreateCloudInstance(ctx, h.Action.GetCreateCloudInstance(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_UpdateCloudInstance:
		err = executeUpdateCloudInstance(ctx, h.Action.GetUpdateCloudInstance(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_DeleteCloudInstance:
		err = executeDeleteCloudInstance(ctx, h.Action.GetDeleteCloudInstance(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_ListCloudInstances:
		err = executeListCloudInstances(ctx, h.Action.GetListCloudInstances(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_ListInstanceConfigs:
		err = executeListInstanceConfigs(ctx, h.Action.GetListInstanceConfigs(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_GetCloudInstanceConfig:
		err = executeGetCloudInstanceConfig(ctx, h.Action.GetGetCloudInstanceConfig(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_GetCloudInstance:
		err = executeGetCloudInstance(ctx, h.Action.GetGetCloudInstance(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_CreateUserInstanceConfig:
		err = executeCreateUserInstanceConfig(ctx, h.Action.GetCreateUserInstanceConfig(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_DeleteUserInstanceConfig:
		err = executeDeleteUserInstanceConfig(ctx, h.Action.GetDeleteUserInstanceConfig(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_CreateCloudDatabase:
		err = executeCreateCloudDatabase(ctx, h.Action.GetCreateCloudDatabase(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_UpdateCloudDatabaseDdl:
		err = executeUpdateCloudDatabaseDdl(ctx, h.Action.GetUpdateCloudDatabaseDdl(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_DropCloudDatabase:
		err = executeDropCloudDatabase(ctx, h.Action.GetDropCloudDatabase(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_CreateCloudBackup:
		err = executeCreateCloudBackup(ctx, h.Action.GetCreateCloudBackup(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_CopyCloudBackup:
		err = executeCopyCloudBackup(ctx, h.Action.GetCopyCloudBackup(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_GetCloudBackup:
		err = executeGetCloudBackup(ctx, h.Action.GetGetCloudBackup(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_UpdateCloudBackup:
		err = executeUpdateCloudBackup(ctx, h.Action.GetUpdateCloudBackup(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_DeleteCloudBackup:
		err = executeDeleteCloudBackup(ctx, h.Action.GetDeleteCloudBackup(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_ListCloudBackups:
		err = executeListCloudBackups(ctx, h.Action.GetListCloudBackups(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_ListCloudBackupOperations:
		err = executeListCloudBackupOperations(ctx, h.Action.GetListCloudBackupOperations(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_ListCloudDatabases:
		err = executeListCloudDatabases(ctx, h.Action.GetListCloudDatabases(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_ListCloudDatabaseOperations:
		err = executeListCloudDatabaseOperations(ctx, h.Action.GetListCloudDatabaseOperations(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_RestoreCloudDatabase:
		err = executeRestoreCloudDatabase(ctx, h.Action.GetRestoreCloudDatabase(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_GetCloudDatabase:
		err = executeGetCloudDatabase(ctx, h.Action.GetGetCloudDatabase(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_GetOperation:
		err = executeGetOperation(ctx, h.Action.GetGetOperation(), h.FlowContext, h.Options, h.OutcomeSender)
	case *executorpb.AdminAction_CancelOperation:
		err = executeCancelOperation(ctx, h.Action.GetCancelOperation(), h.FlowContext, h.Options, h.OutcomeSender)
	default:
		err = spanner.ToSpannerError(status.Error(codes.Unimplemented, fmt.Sprintf("Not implemented yet: %v", h.Action)))
	}
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	return nil
}

// execute action that creates a cloud instance.
func executeCreateCloudInstance(ctx context.Context, action *executorpb.CreateCloudInstanceAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("creating instance:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	op, err := instanceAdminClient.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     fmt.Sprintf("projects/%s", action.GetProjectId()),
		InstanceId: instanceID,
		Instance: &instancepb.Instance{
			Config:          fmt.Sprintf("projects/%s/instanceConfigs/%s", projectID, action.GetInstanceConfigId()),
			DisplayName:     instanceID,
			NodeCount:       action.GetNodeCount(),
			ProcessingUnits: action.GetProcessingUnits(),
			Labels:          action.GetLabels(),
		},
	})
	if err != nil {
		return err
	}
	// Wait for the instance creation to finish.
	_, err = op.Wait(ctx)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that updates a cloud instance.
func executeUpdateCloudInstance(ctx context.Context, action *executorpb.UpdateCloudInstanceAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("updating instance:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	instanceObj := &instancepb.Instance{Name: fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID)}
	var fieldsToUpdate []string
	if action.DisplayName != nil {
		fieldsToUpdate = append(fieldsToUpdate, "display_name")
		instanceObj.DisplayName = instanceID
	}
	if action.NodeCount != nil {
		fieldsToUpdate = append(fieldsToUpdate, "node_count")
		instanceObj.NodeCount = action.GetNodeCount()
	}
	if action.ProcessingUnits != nil {
		fieldsToUpdate = append(fieldsToUpdate, "processing_units")
		instanceObj.ProcessingUnits = action.GetProcessingUnits()
	}
	if action.Labels != nil {
		fieldsToUpdate = append(fieldsToUpdate, "labels")
		instanceObj.Labels = action.GetLabels()
	}

	op, err := instanceAdminClient.UpdateInstance(ctx, &instancepb.UpdateInstanceRequest{
		Instance: instanceObj,
		FieldMask: &fieldmaskpb.FieldMask{
			Paths: fieldsToUpdate,
		},
	})
	if err != nil {
		return err
	}
	// Wait for the instance update to finish.
	_, err = op.Wait(ctx)
	if err != nil {
		return err
	}

	return o.FinishSuccessfully()
}

// execute action that deletes a cloud instance.
func executeDeleteCloudInstance(ctx context.Context, action *executorpb.DeleteCloudInstanceAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("deleting instance:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	err = instanceAdminClient.DeleteInstance(ctx, &instancepb.DeleteInstanceRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
	})
	if err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that lists cloud instances.
func executeListCloudInstances(ctx context.Context, action *executorpb.ListCloudInstancesAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("listing instance:  %v", action)
	projectID := action.GetProjectId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	listInstancesRequest := &instancepb.ListInstancesRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
	}
	if action.PageSize != nil {
		listInstancesRequest.PageSize = action.GetPageSize()
	}
	if action.Filter != nil {
		listInstancesRequest.Filter = action.GetFilter()
	}
	if action.PageToken != nil {
		listInstancesRequest.PageToken = action.GetPageToken()
	}
	iter := instanceAdminClient.ListInstances(ctx, listInstancesRequest)
	var instances []*instancepb.Instance
	for {
		instanceObj, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		instances = append(instances, instanceObj)
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			InstanceResponse: &executorpb.CloudInstanceResponse{
				ListedInstances: instances,
			},
		},
	}
	err = o.SendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

// execute action that lists cloud instance configs.
func executeListInstanceConfigs(ctx context.Context, action *executorpb.ListCloudInstanceConfigsAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("listing instance configs:  %v", action)
	projectID := action.GetProjectId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	listInstanceConfigsRequest := &instancepb.ListInstanceConfigsRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
	}
	if action.PageSize != nil {
		listInstanceConfigsRequest.PageSize = action.GetPageSize()
	}
	if action.PageToken != nil {
		listInstanceConfigsRequest.PageToken = action.GetPageToken()
	}
	iter := instanceAdminClient.ListInstanceConfigs(ctx, listInstanceConfigsRequest)
	var instanceConfigs []*instancepb.InstanceConfig
	for {
		instanceConfig, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		instanceConfigs = append(instanceConfigs, instanceConfig)
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			InstanceConfigResponse: &executorpb.CloudInstanceConfigResponse{
				ListedInstanceConfigs: instanceConfigs,
			},
		},
	}
	err = o.SendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

// execute action that gets a cloud instance config.
func executeGetCloudInstanceConfig(ctx context.Context, action *executorpb.GetCloudInstanceConfigAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("getting instance config:  %v", action)
	projectID := action.GetProjectId()
	instanceConfigID := action.GetInstanceConfigId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	instanceConfig, err := instanceAdminClient.GetInstanceConfig(ctx, &instancepb.GetInstanceConfigRequest{
		Name: fmt.Sprintf("projects/%s/instanceConfigs/%s", projectID, instanceConfigID),
	})
	if err != nil {
		return err
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			InstanceConfigResponse: &executorpb.CloudInstanceConfigResponse{
				InstanceConfig: instanceConfig,
			},
		},
	}
	err = o.SendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

// execute action that retrieves a cloud instance.
func executeGetCloudInstance(ctx context.Context, action *executorpb.GetCloudInstanceAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("retrieving instance:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	instanceObj, err := instanceAdminClient.GetInstance(ctx, &instancepb.GetInstanceRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
	})
	if err != nil {
		return err
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			InstanceResponse: &executorpb.CloudInstanceResponse{
				Instance: instanceObj,
			},
		},
	}
	err = o.SendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

// execute action that creates a user instance config.
func executeCreateUserInstanceConfig(ctx context.Context, action *executorpb.CreateUserInstanceConfigAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("Creating user instance config:  %v", action)
	projectID := action.GetProjectId()
	baseConfigID := action.GetBaseConfigId()
	userConfigID := action.GetUserConfigId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	op, err := instanceAdminClient.CreateInstanceConfig(ctx, &instancepb.CreateInstanceConfigRequest{
		Parent:           fmt.Sprintf("projects/%s", projectID),
		InstanceConfigId: userConfigID,
		InstanceConfig: &instancepb.InstanceConfig{
			Name:        fmt.Sprintf("projects/%s/instanceConfigs/%s", projectID, userConfigID),
			DisplayName: userConfigID,
			Replicas:    action.GetReplicas(),
			BaseConfig:  fmt.Sprintf("projects/%s/instanceConfigs/%s", projectID, baseConfigID),
		},
	})
	if err != nil {
		return err
	}
	_, err = op.Wait(ctx)
	if err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that deletes a user instance config.
func executeDeleteUserInstanceConfig(ctx context.Context, action *executorpb.DeleteUserInstanceConfigAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("deleting user instance config:  %v", action)
	instanceAdminClient, err := instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	err = instanceAdminClient.DeleteInstanceConfig(ctx, &instancepb.DeleteInstanceConfigRequest{
		Name: fmt.Sprintf("projects/%s/instanceConfigs/%s", action.GetProjectId(), action.GetUserConfigId()),
	})
	if err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that creates a cloud database or cloud custom encrypted database.
func executeCreateCloudDatabase(ctx context.Context, action *executorpb.CreateCloudDatabaseAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("creating database:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseID := action.GetDatabaseId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	createDatabaseRequest := &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		CreateStatement: "CREATE DATABASE `" + databaseID + "`",
		ExtraStatements: action.GetSdlStatement(),
	}
	if action.GetEncryptionConfig() != nil {
		createDatabaseRequest.EncryptionConfig = action.GetEncryptionConfig()
	}
	op, err := databaseAdminClient.CreateDatabase(ctx, createDatabaseRequest)
	if err != nil {
		return err
	}
	if _, err := op.Wait(ctx); err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that updates a cloud database.
func executeUpdateCloudDatabaseDdl(ctx context.Context, action *executorpb.UpdateCloudDatabaseDdlAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("updating database ddl %v", action)
	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/%v", action.GetProjectId(), action.GetInstanceId(), action.GetDatabaseId())
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	defer databaseAdminClient.Close()
	op, err := databaseAdminClient.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
		Database:    dbPath,
		Statements:  action.GetSdlStatement(),
		OperationId: action.GetOperationId(),
	})
	if err != nil {
		return fmt.Errorf("UpdateDatabaseDdl: %w", err)
	}
	if err := op.Wait(ctx); err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that drops a cloud database.
func executeDropCloudDatabase(ctx context.Context, action *executorpb.DropCloudDatabaseAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("dropping database:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseID := action.GetDatabaseId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	err = databaseAdminClient.DropDatabase(ctx, &adminpb.DropDatabaseRequest{
		Database: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectID, instanceID, databaseID),
	})
	if err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that creates a cloud database backup.
func executeCreateCloudBackup(ctx context.Context, action *executorpb.CreateCloudBackupAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("creating backup:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseID := action.GetDatabaseId()
	backupID := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	op, err := databaseAdminClient.CreateBackup(ctx, &adminpb.CreateBackupRequest{
		Parent:   fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		BackupId: backupID,
		Backup: &adminpb.Backup{
			Database:    fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectID, instanceID, databaseID),
			VersionTime: action.GetVersionTime(),
			ExpireTime:  action.GetExpireTime(),
		},
	})
	if err != nil {
		return err
	}
	backup, err := op.Wait(ctx)
	if err != nil {
		return err
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			BackupResponse: &executorpb.CloudBackupResponse{
				Backup: backup,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that copies a cloud database backup.
func executeCopyCloudBackup(ctx context.Context, action *executorpb.CopyCloudBackupAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("copying backup:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	backupID := action.GetBackupId()
	sourceBackupID := action.GetSourceBackup()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	op, err := databaseAdminClient.CopyBackup(ctx, &adminpb.CopyBackupRequest{
		Parent:       fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		BackupId:     backupID,
		SourceBackup: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectID, instanceID, sourceBackupID),
		ExpireTime:   action.GetExpireTime(),
	})
	if err != nil {
		return err
	}
	backup, err := op.Wait(ctx)
	if err != nil {
		return err
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			BackupResponse: &executorpb.CloudBackupResponse{
				Backup: backup,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that gets a cloud database backup.
func executeGetCloudBackup(ctx context.Context, action *executorpb.GetCloudBackupAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("getting backup:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	backupID := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	backup, err := databaseAdminClient.GetBackup(ctx, &adminpb.GetBackupRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectID, instanceID, backupID),
	})
	if err != nil {
		return err
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			BackupResponse: &executorpb.CloudBackupResponse{
				Backup: backup,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that updates a cloud database backup.
func executeUpdateCloudBackup(ctx context.Context, action *executorpb.UpdateCloudBackupAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("updating backup:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	backupID := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	backup, err := databaseAdminClient.UpdateBackup(ctx, &adminpb.UpdateBackupRequest{
		Backup: &adminpb.Backup{
			ExpireTime: action.GetExpireTime(),
			Name:       fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectID, instanceID, backupID),
		},
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"expire_time"},
		},
	})
	if err != nil {
		return err
	}
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			BackupResponse: &executorpb.CloudBackupResponse{
				Backup: backup,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that deletes a cloud database backup.
func executeDeleteCloudBackup(ctx context.Context, action *executorpb.DeleteCloudBackupAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("deleting backup:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	backupID := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	err = databaseAdminClient.DeleteBackup(ctx, &adminpb.DeleteBackupRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectID, instanceID, backupID),
	})
	if err != nil {
		return err
	}
	return o.FinishSuccessfully()
}

// execute action that lists cloud database backups.
func executeListCloudBackups(ctx context.Context, action *executorpb.ListCloudBackupsAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("listing backup:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListBackups(ctx, &adminpb.ListBackupsRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		Filter:    action.GetFilter(),
		PageSize:  action.GetPageSize(),
		PageToken: action.GetPageToken(),
	})
	if err != nil {
		return err
	}
	var backupList []*adminpb.Backup
	for {
		backup, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		backupList = append(backupList, backup)
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			BackupResponse: &executorpb.CloudBackupResponse{
				ListedBackups: backupList,
				NextPageToken: "",
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that lists cloud database backup operations.
func executeListCloudBackupOperations(ctx context.Context, action *executorpb.ListCloudBackupOperationsAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("listing backup operation:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListBackupOperations(ctx, &adminpb.ListBackupOperationsRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		Filter:    action.GetFilter(),
		PageSize:  action.GetPageSize(),
		PageToken: action.GetPageToken(),
	})
	if err != nil {
		return err
	}
	var lro []*longrunningpb.Operation
	for {
		operation, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		lro = append(lro, operation)
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			BackupResponse: &executorpb.CloudBackupResponse{
				ListedBackupOperations: lro,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that list cloud databases.
func executeListCloudDatabases(ctx context.Context, action *executorpb.ListCloudDatabasesAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("listing database:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListDatabases(ctx, &adminpb.ListDatabasesRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		PageSize:  action.GetPageSize(),
		PageToken: action.GetPageToken(),
	})
	if err != nil {
		return err
	}
	var databaseList []*adminpb.Database
	for {
		databaseObj, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		databaseList = append(databaseList, databaseObj)
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			DatabaseResponse: &executorpb.CloudDatabaseResponse{
				ListedDatabases: databaseList,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that lists cloud database operations.
func executeListCloudDatabaseOperations(ctx context.Context, action *executorpb.ListCloudDatabaseOperationsAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("listing database operation:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListDatabaseOperations(ctx, &adminpb.ListDatabaseOperationsRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectID, instanceID),
		PageSize:  action.GetPageSize(),
		Filter:    action.GetFilter(),
		PageToken: action.GetPageToken(),
	})
	if err != nil {
		return err
	}
	var lro []*longrunningpb.Operation
	for {
		operation, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		lro = append(lro, operation)
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			DatabaseResponse: &executorpb.CloudDatabaseResponse{
				ListedDatabaseOperations: lro,
				NextPageToken:            "",
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that restores a cloud database.
func executeRestoreCloudDatabase(ctx context.Context, action *executorpb.RestoreCloudDatabaseAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("restoring database:  %v", action)
	projectID := action.GetProjectId()
	databaseInstanceID := action.GetDatabaseInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	restoreOp, err := databaseAdminClient.RestoreDatabase(ctx, &adminpb.RestoreDatabaseRequest{
		Parent:     fmt.Sprintf("projects/%s/instances/%s", projectID, databaseInstanceID),
		DatabaseId: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectID, databaseInstanceID, action.GetDatabaseId()),
		Source: &adminpb.RestoreDatabaseRequest_Backup{
			Backup: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectID, action.GetBackupInstanceId(), action.GetBackupId()),
		},
		EncryptionConfig: nil,
	})
	if err != nil {
		return err
	}
	databaseObject, err := restoreOp.Wait(ctx)
	if err != nil {
		return err
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			DatabaseResponse: &executorpb.CloudDatabaseResponse{
				Database: databaseObject,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that gets a cloud database.
func executeGetCloudDatabase(ctx context.Context, action *executorpb.GetCloudDatabaseAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("getting database:  %v", action)
	projectID := action.GetProjectId()
	instanceID := action.GetInstanceId()
	databaseID := action.GetDatabaseId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	db, err := databaseAdminClient.GetDatabase(ctx, &adminpb.GetDatabaseRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectID, instanceID, databaseID),
	})
	if err != nil {
		return err
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			DatabaseResponse: &executorpb.CloudDatabaseResponse{
				Database: db,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that gets an operation.
func executeGetOperation(ctx context.Context, action *executorpb.GetOperationAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("getting operation:  %v", action)
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	operationResult, err := databaseAdminClient.GetOperation(ctx, &longrunningpb.GetOperationRequest{
		Name: action.GetOperation(),
	})
	if err != nil {
		return err
	}

	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			OperationResponse: &executorpb.OperationResponse{
				Operation: operationResult,
			},
		},
	}
	return o.SendOutcome(spannerActionOutcome)
}

// execute action that cancels an operation.
func executeCancelOperation(ctx context.Context, action *executorpb.CancelOperationAction, h *ExecutionFlowContext, opts []option.ClientOption, o *outputstream.OutcomeSender) error {
	log.Printf("cancelling operation:  %v", action)
	databaseAdminClient, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return err
	}
	err = databaseAdminClient.LROClient.CancelOperation(ctx, &longrunningpb.CancelOperationRequest{
		Name: action.GetOperation(),
	})
	if err != nil {
		return err
	}
	return o.FinishSuccessfully()
}
