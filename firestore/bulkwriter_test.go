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

package firestore

import (
	"context"
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

type bulkwriterTestCase struct {
	name string
	test func(*BulkWriter) (*BulkWriterJob, error)
}

// setupMockServer adds expected write requests and correct mocked responses
// to the mockServer.
func setupMockServer(c *Client, docPrefix string, srv *mockServer) {
	// Create
	srv.addRPC(
		&pb.BatchWriteRequest{
			Database: c.path(),
			Writes: []*pb.Write{
				{
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name:   docPrefix + "a",
							Fields: testFields,
						},
					},
					CurrentDocument: &pb.Precondition{
						ConditionType: &pb.Precondition_Exists{
							Exists: false,
						},
					},
				},
			},
		},
		&pb.BatchWriteResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp},
			},
			Status: []*status.Status{
				{
					Code:    int32(codes.OK),
					Message: "create test successful",
				},
			},
		},
	)

	// Set
	srv.addRPC(
		&pb.BatchWriteRequest{
			Database: c.path(),
			Writes: []*pb.Write{
				{
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name:   docPrefix + "b",
							Fields: testFields,
						},
					},
				},
			},
		},
		&pb.BatchWriteResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp2},
			},
			Status: []*status.Status{
				{
					Code:    int32(codes.OK),
					Message: "set test successful",
				},
			},
		},
	)

	// Delete
	srv.addRPC(
		&pb.BatchWriteRequest{
			Database: c.path(),
			Writes: []*pb.Write{
				{
					Operation: &pb.Write_Delete{
						Delete: docPrefix + "c",
					},
				},
			},
		},
		&pb.BatchWriteResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp3},
			},
			Status: []*status.Status{
				{
					Code:    int32(codes.OK),
					Message: "delete test successful",
				},
			},
		},
	)

	// Update
	srv.addRPC(
		&pb.BatchWriteRequest{
			Database: c.path(),
			Writes: []*pb.Write{
				{
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name:   docPrefix + "f",
							Fields: map[string]*pb.Value{"*": intval(3)},
						},
					},
					UpdateMask: &pb.DocumentMask{FieldPaths: []string{"`*`"}},
					CurrentDocument: &pb.Precondition{
						ConditionType: &pb.Precondition_Exists{
							Exists: true,
						},
					},
				},
			},
		},
		&pb.BatchWriteResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp3},
			},
			Status: []*status.Status{
				{
					Code:    int32(codes.OK),
					Message: "update test successful",
				},
			},
		},
	)
}

func TestBulkWriter(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	docPrefix := c.Collection("C").Path + "/"

	setupMockServer(c, docPrefix, srv)

	ctx := context.Background()
	bw := c.BulkWriter(ctx)
	wantWRs := []*WriteResult{{aTime}, {aTime2}, {aTime3}, {aTime3}}
	tcs := []bulkwriterTestCase{
		{
			name: "Create()",
			test: func(b *BulkWriter) (*BulkWriterJob, error) {
				return b.Create(c.Doc("C/a"), testData)
			},
		},
		{
			name: "Set()",
			test: func(b *BulkWriter) (*BulkWriterJob, error) { return b.Set(c.Doc("C/b"), testData) },
		},
		{
			name: "Delete()",
			test: func(b *BulkWriter) (*BulkWriterJob, error) {
				return b.Delete(c.Doc("C/c"))
			},
		},
		{
			name: "Update()",
			test: func(b *BulkWriter) (*BulkWriterJob, error) {
				return b.Update(c.Doc("C/f"), []Update{{FieldPath: []string{"*"}, Value: 3}})
			},
		},
	}

	for i, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			j, err := tc.test(bw)
			if err != nil {
				t.Errorf("bulkwriter: cannot call %s for document\n", tc.name)
			}
			if j == nil {
				t.Fatalf("bulkwriter: got nil WriteResult for call to %s\n", tc.name)
			}

			bw.Flush()

			wr, err := j.Results()
			if err != nil {
				t.Errorf("bulkwriter:\nwanted %v,\n, got error: %v", wantWRs[i], err)
			}

			if !testEqual(wr, wantWRs[i]) {
				t.Errorf("bulkwriter:\nwanted %v,\n got %v\n", wantWRs[i], wr)
			}
		})
	}
}

func TestBulkWriterErrors(t *testing.T) {
	c, _, cleanup := newMock(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	b := c.BulkWriter(ctx)

	tcs := []bulkwriterTestCase{
		{
			name: "empty document reference",
			test: func(b *BulkWriter) (*BulkWriterJob, error) {
				return b.Delete(nil)
			},
		},
		{
			name: "cannot write to same document twice",
			test: func(b *BulkWriter) (*BulkWriterJob, error) {
				b.Create(c.Doc("C/a"), testData)
				return b.Delete(c.Doc("C/a"))
			},
		},
		{
			name: "cannot ask a closed BulkWriter to write",
			test: func(b *BulkWriter) (*BulkWriterJob, error) {
				cancel()
				b.End()
				return b.Delete(c.Doc("C/b"))
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.test(b)
			if err == nil {
				t.Fatalf("wanted error, got nil value")
			}
		})
	}
}
