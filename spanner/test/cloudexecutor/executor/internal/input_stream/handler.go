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

package input_stream

// input_stream_handler.go is responsible for handling input requests to the server and
// handles mapping from executor actions (SpannerAsyncActionRequest) to client library code.

import (
	"context"
	"sync"

	executorpb "cloud.google.com/go/spanner/test/cloudexecutor/proto"
	"google.golang.org/api/option"
)

type CloudStreamHandler struct {
	// members below should be set by the caller
	Stream        executorpb.SpannerExecutorProxy_ExecuteActionAsyncServer
	ServerContext context.Context
	Options       []option.ClientOption
	// members below represent internal state
	mu sync.Mutex // protects mutable internal state
}

// Execute executes the given ExecuteActions request, blocking until it's done. It takes care of
// properly closing the request stream in the end.
func (h *CloudStreamHandler) Execute() error {
	return nil
}
