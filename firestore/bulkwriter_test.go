package firestore

import (
	"testing"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

type bulkwriterTestCase struct {
	name string
	test func(*BulkWriter) (*BulkWriterJob, error)
}

func TestBulkWriter(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	docPrefix := c.Collection("C").Path + "/"

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

	bw := c.BulkWriter()
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
				t.Errorf("bulkwriter: got nil WriteResult for call to %s\n", tc.name)
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
