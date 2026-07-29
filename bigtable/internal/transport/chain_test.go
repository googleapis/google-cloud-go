// Copyright 2026 Google LLC
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

package internal

import (
	"context"
	"testing"
)

func TestChainedInterceptors_Success(t *testing.T) {
	var order []string

	int1 := func(ctx context.Context, req interface{}, next Handler) (interface{}, error) {
		order = append(order, "int1_start")
		res, err := next(ctx, req)
		order = append(order, "int1_end")
		return res, err
	}

	int2 := func(ctx context.Context, req interface{}, next Handler) (interface{}, error) {
		order = append(order, "int2_start")
		res, err := next(ctx, req)
		order = append(order, "int2_end")
		return res, err
	}

	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		order = append(order, "base")
		return "response", nil
	}

	chained := ChainInterceptors(int1, int2)
	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)

	resp, err := chained(ctx, "request", baseHandler)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp.(string) != "response" {
		t.Errorf("Expected response to be 'response', got %v", resp)
	}

	expectedOrder := []string{"int1_start", "int2_start", "base", "int2_end", "int1_end"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("Expected order length %d, got %d", len(expectedOrder), len(order))
	}
	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("At index %d: expected %q, got %q", i, v, order[i])
		}
	}
}
