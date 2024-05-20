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

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WriteActionHandler holds the necessary components required for Write action.
type WriteActionHandler struct {
	Action        *executorpb.MutationAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction that execute a Write action request.
func (h *WriteActionHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("executing write action: %v", h.Action)
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()

	m, err := createMutation(h.Action, h.FlowContext.tableMetadata)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}

	_, err = h.FlowContext.DbClient.Apply(ctx, m)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	return h.OutcomeSender.FinishSuccessfully()
}

// MutationActionHandler holds the necessary components required for Mutation action.
type MutationActionHandler struct {
	Action        *executorpb.MutationAction
	FlowContext   *ExecutionFlowContext
	OutcomeSender *outputstream.OutcomeSender
}

// ExecuteAction that execute a Mutation action request.
func (h *MutationActionHandler) ExecuteAction(ctx context.Context) error {
	log.Printf("Buffering mutation %v", h.Action)
	h.FlowContext.mu.Lock()
	defer h.FlowContext.mu.Unlock()
	txn, err := h.FlowContext.getTransactionForWrite()
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	m, err := createMutation(h.Action, h.FlowContext.tableMetadata)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}

	err = txn.BufferWrite(m)
	if err != nil {
		return h.OutcomeSender.FinishWithError(err)
	}
	return h.OutcomeSender.FinishSuccessfully()
}

// createMutation creates cloud spanner.Mutation from given executorpb.MutationAction.
func createMutation(action *executorpb.MutationAction, tableMetadata *utility.TableMetadataHelper) ([]*spanner.Mutation, error) {
	prevTable := ""
	var m []*spanner.Mutation
	for _, mod := range action.Mod {
		table := mod.GetTable()
		if table == "" {
			table = prevTable
		}
		if table == "" {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, fmt.Sprintf("table name is missing from mod: action %s ", action.String())))
		}
		prevTable = table
		log.Printf("executing mutation mod: \n%s", mod.String())

		switch {
		case mod.Insert != nil:
			ia := mod.Insert
			cloudRows, err := cloudValuesFromExecutorValueLists(ia.GetValues(), ia.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.Insert(table, ia.GetColumn(), cloudRow))
			}
		case mod.Update != nil:
			ua := mod.Update
			cloudRows, err := cloudValuesFromExecutorValueLists(ua.GetValues(), ua.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.Update(table, ua.GetColumn(), cloudRow))
			}
		case mod.InsertOrUpdate != nil:
			ia := mod.InsertOrUpdate
			cloudRows, err := cloudValuesFromExecutorValueLists(ia.GetValues(), ia.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.InsertOrUpdate(table, ia.GetColumn(), cloudRow))
			}
		case mod.Replace != nil:
			ia := mod.Replace
			cloudRows, err := cloudValuesFromExecutorValueLists(ia.GetValues(), ia.GetType())
			if err != nil {
				return nil, err
			}
			for _, cloudRow := range cloudRows {
				m = append(m, spanner.Replace(table, ia.GetColumn(), cloudRow))
			}
		case mod.DeleteKeys != nil:
			keyColTypes, err := tableMetadata.GetKeyColumnTypes(table)
			if err != nil {
				return nil, err
			}
			keySet, err := utility.KeySetProtoToCloudKeySet(mod.DeleteKeys, keyColTypes)
			if err != nil {
				return nil, err
			}
			m = append(m, spanner.Delete(table, keySet))
		default:
			return nil, spanner.ToSpannerError(status.Errorf(codes.InvalidArgument, "unsupported mod: %s", mod.String()))
		}
	}
	return m, nil
}

// cloudValuesFromExecutorValueLists produces rows of Cloud Spanner values given []*executorpb.ValueList and []*spannerpb.Type.
// Each ValueList results in a row, and all of them should have the same column types.
func cloudValuesFromExecutorValueLists(valueLists []*executorpb.ValueList, types []*spannerpb.Type) ([][]any, error) {
	var cloudRows [][]any
	for _, rowValues := range valueLists {
		log.Printf("Converting ValueList: %s\n", rowValues)
		if len(rowValues.GetValue()) != len(types) {
			return nil, spanner.ToSpannerError(status.Error(codes.InvalidArgument, "number of values should be equal to number of types"))
		}

		var cloudRow []any
		for i, v := range rowValues.GetValue() {
			isNull := false
			switch v.GetValueType().(type) {
			case *executorpb.Value_IsNull:
				isNull = true
			}
			val, err := utility.ExecutorValueToSpannerValue(types[i], v, isNull)
			if err != nil {
				return nil, err
			}
			cloudRow = append(cloudRow, val)
		}
		cloudRows = append(cloudRows, cloudRow)
	}
	return cloudRows, nil
}
