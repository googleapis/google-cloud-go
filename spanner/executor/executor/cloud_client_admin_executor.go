package executor

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
	executorpb "cloud.google.com/go/spanner/executor/proto"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type adminActionHandler struct {
	action        *executorpb.AdminAction
	context       context.Context
	flowContext   *executionFlowContext
	outcomeSender *outcomeSender
	options       []option.ClientOption
}

func (h *adminActionHandler) executeAction(ctx context.Context) error {
	log.Printf("executing admin action %v", h.action)
	h.flowContext.mu.Lock()
	defer h.flowContext.mu.Unlock()
	var err error
	switch h.action.GetAction().(type) {
	case *executorpb.AdminAction_CreateCloudInstance:
		err = executeCreateCloudInstance(h.action.GetCreateCloudInstance(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_UpdateCloudInstance:
		err = executeUpdateCloudInstance(h.action.GetUpdateCloudInstance(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_DeleteCloudInstance:
		err = executeDeleteCloudInstance(h.action.GetDeleteCloudInstance(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_ListCloudInstances:
		err = executeListCloudInstances(h.action.GetListCloudInstances(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_ListInstanceConfigs:
		err = executeListInstanceConfigs(h.action.GetListInstanceConfigs(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_GetCloudInstanceConfig:
		err = executeGetCloudInstanceConfig(h.action.GetGetCloudInstanceConfig(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_GetCloudInstance:
		err = executeGetCloudInstance(h.action.GetGetCloudInstance(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_CreateUserInstanceConfig:
		err = executeCreateUserInstanceConfig(h.action.GetCreateUserInstanceConfig(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_DeleteUserInstanceConfig:
		err = executeDeleteUserInstanceConfig(h.action.GetDeleteUserInstanceConfig(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_CreateCloudDatabase:
		err = executeCreateCloudDatabase(h.action.GetCreateCloudDatabase(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_UpdateCloudDatabaseDdl:
		err = executeUpdateCloudDatabaseDdl(h.action.GetUpdateCloudDatabaseDdl(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_DropCloudDatabase:
		err = executeDropCloudDatabase(h.action.GetDropCloudDatabase(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_CreateCloudBackup:
		err = executeCreateCloudBackup(h.action.GetCreateCloudBackup(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_CopyCloudBackup:
		err = executeCopyCloudBackup(h.action.GetCopyCloudBackup(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_GetCloudBackup:
		err = executeGetCloudBackup(h.action.GetGetCloudBackup(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_UpdateCloudBackup:
		err = executeUpdateCloudBackup(h.action.GetUpdateCloudBackup(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_DeleteCloudBackup:
		err = executeDeleteCloudBackup(h.action.GetDeleteCloudBackup(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_ListCloudBackups:
		err = executeListCloudBackups(h.action.GetListCloudBackups(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_ListCloudBackupOperations:
		err = executeListCloudBackupOperations(h.action.GetListCloudBackupOperations(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_ListCloudDatabases:
		err = executeListCloudDatabases(h.action.GetListCloudDatabases(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_ListCloudDatabaseOperations:
		err = executeListCloudDatabaseOperations(h.action.GetListCloudDatabaseOperations(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_RestoreCloudDatabase:
		err = executeRestoreCloudDatabase(h.action.GetRestoreCloudDatabase(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_GetCloudDatabase:
		err = executeGetCloudDatabase(h.action.GetGetCloudDatabase(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_GetOperation:
		err = executeGetOperation(h.action.GetGetOperation(), h.flowContext, h.options, h.outcomeSender)
	case *executorpb.AdminAction_CancelOperation:
		err = executeCancelOperation(h.action.GetCancelOperation(), h.flowContext, h.options, h.outcomeSender)
	default:
		err = spanner.ToSpannerError(status.Error(codes.Unimplemented, fmt.Sprintf("Not implemented yet: %v", h.action)))
	}
	if err != nil {
		return h.outcomeSender.finishWithError(err)
	}
	//return h.outcomeSender.finishSuccessfully()
	return nil
}

func executeUpdateCloudDatabaseDdl(action *executorpb.UpdateCloudDatabaseDdlAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("updating database ddl %v", action)
	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/%v", action.GetProjectId(), action.GetInstanceId(), action.GetDatabaseId())
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer databaseAdminClient.Close()
	op, err := databaseAdminClient.UpdateDatabaseDdl(h.txnContext, &adminpb.UpdateDatabaseDdlRequest{
		Database:    dbPath,
		Statements:  action.GetSdlStatement(),
		OperationId: action.GetOperationId(),
	})
	if err != nil {
		return fmt.Errorf("UpdateDatabaseDdl: %w", err)
	}
	if err := op.Wait(h.txnContext); err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeCreateCloudInstance(action *executorpb.CreateCloudInstanceAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("creating instance:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	op, err := instanceAdminClient.CreateInstance(h.txnContext, &instancepb.CreateInstanceRequest{
		Parent:     fmt.Sprintf("projects/%s", action.GetProjectId()),
		InstanceId: instanceId,
		Instance: &instancepb.Instance{
			Config:          fmt.Sprintf("projects/%s/instanceConfigs/%s", projectId, action.GetInstanceConfigId()),
			DisplayName:     instanceId,
			NodeCount:       action.GetNodeCount(),
			ProcessingUnits: action.GetProcessingUnits(),
			Labels:          action.GetLabels(),
		},
	})
	if err != nil {
		return err
	}
	// Wait for the instance creation to finish.
	_, err = op.Wait(h.txnContext)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return err
	}

	return o.finishSuccessfully()
}

func executeUpdateCloudInstance(action *executorpb.UpdateCloudInstanceAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("updating instance:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	instanceObj := &instancepb.Instance{Name: fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId)}
	var fieldsToUpdate []string
	if action.DisplayName != nil {
		fieldsToUpdate = append(fieldsToUpdate, "display_name")
		instanceObj.DisplayName = instanceId
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

	op, err := instanceAdminClient.UpdateInstance(h.txnContext, &instancepb.UpdateInstanceRequest{
		Instance: instanceObj,
		FieldMask: &fieldmaskpb.FieldMask{
			Paths: fieldsToUpdate,
		},
	})
	if err != nil {
		return err
	}
	// Wait for the instance update to finish.
	_, err = op.Wait(h.txnContext)
	if err != nil {
		return err
	}

	return o.finishSuccessfully()
}

func executeDeleteCloudInstance(action *executorpb.DeleteCloudInstanceAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("deleting instance:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	err = instanceAdminClient.DeleteInstance(h.txnContext, &instancepb.DeleteInstanceRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
	})
	if err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeListCloudInstances(action *executorpb.ListCloudInstancesAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("listing instance:  %v", action)
	projectId := action.GetProjectId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	listInstancesRequest := &instancepb.ListInstancesRequest{
		Parent: fmt.Sprintf("projects/%s", projectId),
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
	iter := instanceAdminClient.ListInstances(h.txnContext, listInstancesRequest)
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeListInstanceConfigs(action *executorpb.ListCloudInstanceConfigsAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("listing instance configs:  %v", action)
	projectId := action.GetProjectId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	listInstanceConfigsRequest := &instancepb.ListInstanceConfigsRequest{
		Parent: fmt.Sprintf("projects/%s", projectId),
	}
	if action.PageSize != nil {
		listInstanceConfigsRequest.PageSize = action.GetPageSize()
	}
	if action.PageToken != nil {
		listInstanceConfigsRequest.PageToken = action.GetPageToken()
	}
	iter := instanceAdminClient.ListInstanceConfigs(h.txnContext, listInstanceConfigsRequest)
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeGetCloudInstanceConfig(action *executorpb.GetCloudInstanceConfigAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("getting instance config:  %v", action)
	projectId := action.GetProjectId()
	instanceConfigId := action.GetInstanceConfigId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	instanceConfig, err := instanceAdminClient.GetInstanceConfig(h.txnContext, &instancepb.GetInstanceConfigRequest{
		Name: fmt.Sprintf("projects/%s/instanceConfigs/%s", projectId, instanceConfigId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeGetCloudInstance(action *executorpb.GetCloudInstanceAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("retrieving instance:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	defer instanceAdminClient.Close()
	instanceObj, err := instanceAdminClient.GetInstance(h.txnContext, &instancepb.GetInstanceRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
	})
	spannerActionOutcome := &executorpb.SpannerActionOutcome{
		Status: &spb.Status{Code: int32(codes.OK)},
		AdminResult: &executorpb.AdminResult{
			InstanceResponse: &executorpb.CloudInstanceResponse{
				Instance: instanceObj,
			},
		},
	}
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeCreateUserInstanceConfig(action *executorpb.CreateUserInstanceConfigAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("Creating user instance config:  %v", action)
	projectId := action.GetProjectId()
	baseConfigId := action.GetBaseConfigId()
	userConfigId := action.GetUserConfigId()
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	op, err := instanceAdminClient.CreateInstanceConfig(h.txnContext, &instancepb.CreateInstanceConfigRequest{
		Parent:           fmt.Sprintf("projects/%s", projectId),
		InstanceConfigId: userConfigId,
		InstanceConfig: &instancepb.InstanceConfig{
			Name:        fmt.Sprintf("projects/%s/instanceConfigs/%s", projectId, userConfigId),
			DisplayName: userConfigId,
			Replicas:    action.GetReplicas(),
			BaseConfig:  fmt.Sprintf("projects/%s/instanceConfigs/%s", projectId, baseConfigId),
		},
	})
	if err != nil {
		return err
	}
	_, err = op.Wait(h.txnContext)
	if err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeDeleteUserInstanceConfig(action *executorpb.DeleteUserInstanceConfigAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("deleting user instance config:  %v", action)
	instanceAdminClient, err := instance.NewInstanceAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	err = instanceAdminClient.DeleteInstanceConfig(h.txnContext, &instancepb.DeleteInstanceConfigRequest{
		Name: fmt.Sprintf("projects/%s/instanceConfigs/%s", action.GetProjectId(), action.GetUserConfigId()),
	})
	if err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeCreateCloudDatabase(action *executorpb.CreateCloudDatabaseAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("creating database:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseId := action.GetDatabaseId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	createDatabaseRequest := &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
		CreateStatement: "CREATE DATABASE `" + databaseId + "`",
		ExtraStatements: action.GetSdlStatement(),
	}
	if action.GetEncryptionConfig() != nil {
		createDatabaseRequest.EncryptionConfig = action.GetEncryptionConfig()
	}
	op, err := databaseAdminClient.CreateDatabase(h.txnContext, createDatabaseRequest)
	if err != nil {
		return err
	}
	if _, err := op.Wait(h.txnContext); err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeDropCloudDatabase(action *executorpb.DropCloudDatabaseAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("dropping database:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseId := action.GetDatabaseId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	err = databaseAdminClient.DropDatabase(h.txnContext, &adminpb.DropDatabaseRequest{
		Database: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
	})
	if err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeCreateCloudBackup(action *executorpb.CreateCloudBackupAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("creating backup:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseId := action.GetDatabaseId()
	backupId := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	op, err := databaseAdminClient.CreateBackup(h.txnContext, &adminpb.CreateBackupRequest{
		Parent:   fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
		BackupId: backupId,
		Backup: &adminpb.Backup{
			Database:    fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
			VersionTime: action.GetVersionTime(),
			ExpireTime:  action.GetExpireTime(),
		},
	})
	if err != nil {
		return err
	}
	backup, err := op.Wait(h.txnContext)
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeCopyCloudBackup(action *executorpb.CopyCloudBackupAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("copying backup:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	backupId := action.GetBackupId()
	sourceBackupId := action.GetSourceBackup()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	op, err := databaseAdminClient.CopyBackup(h.txnContext, &adminpb.CopyBackupRequest{
		Parent:       fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
		BackupId:     backupId,
		SourceBackup: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectId, instanceId, sourceBackupId),
		ExpireTime:   action.GetExpireTime(),
	})
	if err != nil {
		return err
	}
	backup, err := op.Wait(h.txnContext)
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeGetCloudBackup(action *executorpb.GetCloudBackupAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("getting backup:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	backupId := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	backup, err := databaseAdminClient.GetBackup(h.txnContext, &adminpb.GetBackupRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectId, instanceId, backupId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeUpdateCloudBackup(action *executorpb.UpdateCloudBackupAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("updating backup:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	backupId := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	backup, err := databaseAdminClient.UpdateBackup(h.txnContext, &adminpb.UpdateBackupRequest{
		Backup: &adminpb.Backup{
			ExpireTime: action.GetExpireTime(),
			Name:       fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectId, instanceId, backupId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeDeleteCloudBackup(action *executorpb.DeleteCloudBackupAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("deleting backup:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	backupId := action.GetBackupId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	err = databaseAdminClient.DeleteBackup(h.txnContext, &adminpb.DeleteBackupRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectId, instanceId, backupId),
	})
	if err != nil {
		return err
	}
	return o.finishSuccessfully()
}

func executeListCloudBackups(action *executorpb.ListCloudBackupsAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("listing backup:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListBackups(h.txnContext, &adminpb.ListBackupsRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeListCloudBackupOperations(action *executorpb.ListCloudBackupOperationsAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("listing backup operation:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListBackupOperations(h.txnContext, &adminpb.ListBackupOperationsRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeListCloudDatabases(action *executorpb.ListCloudDatabasesAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("listing database:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListDatabases(h.txnContext, &adminpb.ListDatabasesRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeListCloudDatabaseOperations(action *executorpb.ListCloudDatabaseOperationsAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("listing database operation:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	iter := databaseAdminClient.ListDatabaseOperations(h.txnContext, &adminpb.ListDatabaseOperationsRequest{
		Parent:    fmt.Sprintf("projects/%s/instances/%s", projectId, instanceId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeRestoreCloudDatabase(action *executorpb.RestoreCloudDatabaseAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("restoring database:  %v", action)
	projectId := action.GetProjectId()
	databaseInstanceId := action.GetDatabaseInstanceId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	restoreOp, err := databaseAdminClient.RestoreDatabase(h.txnContext, &adminpb.RestoreDatabaseRequest{
		Parent:     fmt.Sprintf("projects/%s/instances/%s", projectId, databaseInstanceId),
		DatabaseId: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, databaseInstanceId, action.GetDatabaseId()),
		Source: &adminpb.RestoreDatabaseRequest_Backup{
			Backup: fmt.Sprintf("projects/%s/instances/%s/backups/%s", projectId, action.GetBackupInstanceId(), action.GetBackupId()),
		},
		EncryptionConfig: nil,
	})
	if err != nil {
		return err
	}
	databaseObject, err := restoreOp.Wait(h.txnContext)
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeGetCloudDatabase(action *executorpb.GetCloudDatabaseAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("getting database:  %v", action)
	projectId := action.GetProjectId()
	instanceId := action.GetInstanceId()
	databaseId := action.GetDatabaseId()
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	db, err := databaseAdminClient.GetDatabase(h.txnContext, &adminpb.GetDatabaseRequest{
		Name: fmt.Sprintf("projects/%s/instances/%s/databases/%s", projectId, instanceId, databaseId),
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeGetOperation(action *executorpb.GetOperationAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("getting operation:  %v", action)
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	operationResult, err := databaseAdminClient.GetOperation(h.txnContext, &longrunningpb.GetOperationRequest{
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
	err = o.sendOutcome(spannerActionOutcome)
	if err != nil {
		return err
	}
	return nil
}

func executeCancelOperation(action *executorpb.CancelOperationAction, h *executionFlowContext, opts []option.ClientOption, o *outcomeSender) error {
	log.Printf("cancelling operation:  %v", action)
	databaseAdminClient, err := database.NewDatabaseAdminClient(h.txnContext, opts...)
	if err != nil {
		return err
	}
	err = databaseAdminClient.LROClient.CancelOperation(h.txnContext, &longrunningpb.CancelOperationRequest{
		Name: action.GetOperation(),
	})
	if err != nil {
		return err
	}
	return o.finishSuccessfully()
}
