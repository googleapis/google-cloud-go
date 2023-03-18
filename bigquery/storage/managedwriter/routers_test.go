// Copyright 2023 Google LLC
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

package managedwriter

import (
	"context"
	"testing"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
)

func TestSimpleRouter(t *testing.T) {

	ctx := context.Background()

	pool := &connectionPool{
		ctx: ctx,
		open: func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
			return &testAppendRowsClient{}, nil
		},
	}

	router := newSimpleRouter("")
	if err := pool.activateRouter(router); err != nil {
		t.Errorf("activateRouter: %v", err)
	}

	ms := &ManagedStream{
		ctx:   ctx,
		retry: newStatelessRetryer(),
	}

	pw := newPendingWrite(ctx, ms, &storagepb.AppendRowsRequest{}, nil, "", "")

	// picking before attaching should yield error
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
	writer := &ManagedStream{
		id: "writer",
	}
	if err := pool.addWriter(writer); err != nil {
		t.Errorf("addWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err != nil {
		t.Errorf("pickConnection error: %v", err)
	}
	if err := pool.removeWriter(writer); err != nil {
		t.Errorf("disconnectWriter: %v", err)
	}
	if _, err := pool.router.pickConnection(pw); err == nil {
		t.Errorf("pickConnection: expected error, got success")
	}
}
