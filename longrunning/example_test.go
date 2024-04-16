// Copyright 2016 Google LLC
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

package longrunning

import (
	"context"
	"fmt"
	"time"

	pb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func bestMomentInHistory() (*Operation, error) {
	t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", "2009-11-10 23:00:00 +0000 UTC")
	if err != nil {
		return nil, err
	}
	resp := timestamppb.New(t)
	respAny, err := anypb.New(resp)
	if err != nil {
		return nil, err
	}
	metaAny, err := anypb.New(durationpb.New(1 * time.Hour))
	return &Operation{
		proto: &pb.Operation{
			Name:     "best-moment",
			Done:     true,
			Metadata: metaAny,
			Result: &pb.Operation_Response{
				Response: respAny,
			},
		},
	}, err
}

func ExampleOperation_Wait() {
	// Complex computation, might take a long time.
	op, err := bestMomentInHistory()
	if err != nil {
		// TODO: Handle err.
	}
	var ts timestamppb.Timestamp
	err = op.Wait(context.TODO(), &ts)
	if err != nil && !op.Done() {
		fmt.Println("failed to fetch operation status", err)
	} else if err != nil && op.Done() {
		fmt.Println("operation completed with error", err)
	} else {
		fmt.Println(ts.AsTime().Format(time.RFC3339Nano))
	}
	// Output:
	// 2009-11-10T23:00:00Z
}

func ExampleOperation_Metadata() {
	op, err := bestMomentInHistory()
	if err != nil {
		// TODO: Handle err.
	}

	// The operation might contain metadata.
	// In this example, the metadata contains the estimated length of time
	// the operation might take to complete.
	var meta durationpb.Duration
	if err := op.Metadata(&meta); err != nil {
		// TODO: Handle err.
	}
	if err := meta.CheckValid(); err == ErrNoMetadata {
		fmt.Println("no metadata")
		return
	} else if err != nil {
		// TODO: Handle err.
		return
	}
	fmt.Println(meta.AsDuration())

	// Output:
	// 1h0m0s
}

func ExampleOperation_Cancel() {
	op, err := bestMomentInHistory()
	if err != nil {
		// TODO: Handle err.
	}
	if err := op.Cancel(context.Background()); err != nil {
		// TODO: Handle err.
	}
}

func ExampleOperation_Delete() {
	op, err := bestMomentInHistory()
	if err != nil {
		// TODO: Handle err.
	}
	if err := op.Delete(context.Background()); err != nil {
		// TODO: Handle err.
	}
}
