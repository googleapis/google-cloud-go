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

package inputstream

// input_stream_handler.go is responsible for handling input requests to the server and
// handles mapping from executor actions (SpannerAsyncActionRequest) to client library code.

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/executor/apiv1/executorpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/actions"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CloudStreamHandler handles a streaming ExecuteActions request by performing incoming
// actions. It maintains a state associated with the request, such as current transaction.
type CloudStreamHandler struct {
	// members below should be set by the caller
	Stream        executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer
	ServerContext context.Context
	Options       []option.ClientOption
	// members below represent internal state
	executionFlowContext *actions.ExecutionFlowContext
	mu                   sync.Mutex // protects mutable internal state
}

// Execute executes the given ExecuteActions request, blocking until it's done. It takes care of
// properly closing the request stream in the end.
func (h *CloudStreamHandler) Execute() error {
	log.Println("ExecuteActionAsync RPC called. Start handling input stream")

	// Enable UseNumberWithJSONDecoderEncoder so that JSON numbers are decoded
	// as Number (preserving precision) and not float64 (risking loss).
	spanner.UseNumberWithJSONDecoderEncoder(true)

	var c *actions.ExecutionFlowContext
	func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		c = &actions.ExecutionFlowContext{}
		h.executionFlowContext = c
	}()

	// In case this function returns abruptly, or client misbehaves, make sure to dispose of
	// transactions.
	defer func() {
		c.CloseOpenTransactions()
	}()

	ctx := context.Background()
	// Main loop that receives and executes actions.
	for {
		req, err := h.Stream.Recv()
		if err == io.EOF {
			log.Println("Client called Done, half-closed the stream")
			if h.executionFlowContext != nil && h.executionFlowContext.DbClient != nil {
				log.Println("Closing the client object in execution flow context")
				h.executionFlowContext.DbClient.Close()
			}
			break
		}
		if err != nil {
			log.Printf("Failed to receive request from client: %v", err)
			return err
		}
		if err = h.startHandlingRequest(ctx, req); err != nil {
			log.Printf("Failed to handle request %v, Client ends the stream with error: %v", req, err)
			// TODO(sriharshach): should we throw the error here instead of nil?
			return nil
		}
	}
	log.Println("Done executing actions")
	return nil
}

// startHandlingRequest takes care of the given request. It picks an actionHandler and starts
// a goroutine in which that action will be executed.
func (h *CloudStreamHandler) startHandlingRequest(ctx context.Context, req *executorpb.SpannerAsyncActionRequest) error {
	log.Printf("start handling request %v", req)
	h.mu.Lock()
	defer h.mu.Unlock()

	outcomeSender := outputstream.NewOutcomeSender(req.GetActionId(), h.Stream)

	inputAction := req.GetAction()
	if inputAction == nil {
		log.Println("Invalid SpannerAsyncActionRequest, input action is nil")
		return outcomeSender.FinishWithError(spanner.ToSpannerError(status.Error(codes.InvalidArgument, "Invalid request")))
	}

	// Get a new action handler based on the input action.
	actionHandler, err := h.newActionHandler(inputAction, outcomeSender)
	if err != nil {
		return outcomeSender.FinishWithError(err)
	}

	// Create a channel to receive the error from the goroutine.
	errCh := make(chan error, 1)
	successCh := make(chan bool, 1)

	go func() {
		err := actionHandler.ExecuteAction(ctx)
		if err != nil {
			log.Printf("Failed to execute action with error %v: %v", inputAction, err)
			errCh <- err
		} else {
			successCh <- true
		}
	}()

	// Wait for the goroutine to finish or return an error if one occurs.
	select {
	case err := <-errCh:
		// An error occurred in the goroutine.
		log.Printf("Client ends the stream with error. Failed to execute action %v with error: %v", inputAction, err)
		return err
	case <-successCh:
		// Success signal received.
		log.Println("Action executed successfully")
		return nil
	}
}

// newActionHandler instantiates an actionHandler for executing the given action.
func (h *CloudStreamHandler) newActionHandler(action *executorpb.SpannerAction, outcomeSender *outputstream.OutcomeSender) (cloudActionHandler, error) {
	if action.DatabasePath != "" {
		h.executionFlowContext.Database = action.DatabasePath
	}
	switch action.GetAction().(type) {
	case *executorpb.SpannerAction_Start:
		return &actions.StartTxnHandler{
			Action:        action.GetStart(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
			Options:       h.Options,
		}, nil
	case *executorpb.SpannerAction_Finish:
		return &actions.FinishTxnHandler{
			Action:        action.GetFinish(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Admin:
		adminAction := &actions.AdminActionHandler{
			Action:        action.GetAdmin(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
			Options:       h.Options,
		}
		return adminAction, nil
	case *executorpb.SpannerAction_Read:
		return &actions.ReadActionHandler{
			Action:        action.GetRead(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Query:
		return &actions.QueryActionHandler{
			Action:        action.GetQuery(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Mutation:
		return &actions.MutationActionHandler{
			Action:        action.GetMutation(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Write:
		return &actions.WriteActionHandler{
			Action:        action.GetWrite().GetMutation(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_Dml:
		return &actions.DmlActionHandler{
			Action:        action.GetDml(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_StartBatchTxn:
		return &actions.StartBatchTxnHandler{
			Action:        action.GetStartBatchTxn(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
			Options:       h.Options,
		}, nil
	case *executorpb.SpannerAction_GenerateDbPartitionsRead:
		return &actions.PartitionReadActionHandler{
			Action:        action.GetGenerateDbPartitionsRead(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_GenerateDbPartitionsQuery:
		return &actions.PartitionQueryActionHandler{
			Action:        action.GetGenerateDbPartitionsQuery(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_ExecutePartition:
		return &actions.ExecutePartition{
			Action:        action.GetExecutePartition(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_PartitionedUpdate:
		return &actions.PartitionedUpdate{
			Action:        action.GetPartitionedUpdate(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_CloseBatchTxn:
		return &actions.CloseBatchTxnHandler{
			Action:        action.GetCloseBatchTxn(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	case *executorpb.SpannerAction_BatchDml:
		return &actions.BatchDmlHandler{
			Action:        action.GetBatchDml(),
			FlowContext:   h.executionFlowContext,
			OutcomeSender: outcomeSender,
		}, nil
	default:
		return nil, status.Error(codes.Unimplemented, fmt.Sprintf("not implemented yet %T", action.GetAction()))
	}
}

// cloudActionHandler is an interface representing an entity responsible for executing a particular
// kind of SpannerActions.
type cloudActionHandler interface {
	ExecuteAction(context.Context) error
}
