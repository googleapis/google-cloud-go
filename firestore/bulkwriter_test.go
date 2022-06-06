package firestore

import (
	"testing"
	"time"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

type bulkwriterWriteTestCase func(*BulkWriter, chan *WriteResult, chan *error)

func bulkwriterTestRunner(bw *BulkWriter, f bulkwriterWriteTestCase) (*WriteResult, error) {
	pwp := bw.Status().WritesProvidedCount
	pwr := bw.Status().WritesReceivedCount

	wr := make(chan *WriteResult, 1)
	err := make(chan *error, 1)
	go f(bw, wr, err)

	for bw.Status().WritesProvidedCount != pwp+1 {
		time.Sleep(time.Duration(time.Millisecond))
	}

	bw.Flush()

	for bw.Status().WritesReceivedCount != pwr+1 {
		time.Sleep(time.Duration(time.Millisecond))
	}

	return <-wr, *(<-err)
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
						ConditionType: &pb.Precondition_Exists{false},
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
						ConditionType: &pb.Precondition_Exists{true},
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

	var tcs []bulkwriterWriteTestCase
	wantWRs := []*WriteResult{{aTime}, {aTime2}, {aTime3}, {aTime3}}

	var testCreateCase bulkwriterWriteTestCase
	testCreateCase = func(bw *BulkWriter, wr chan *WriteResult, err chan *error) {
		wrc, errs := bw.Create(c.Doc("C/a"), testData)
		if errs != nil {
			wr <- nil
			err <- &errs
			return
		}
		wr <- wrc
		err <- &errs
	}
	tcs = append(tcs, testCreateCase)

	var testSetCase bulkwriterWriteTestCase
	testSetCase = func(bw *BulkWriter, wr chan *WriteResult, err chan *error) {
		wrs, errs := bw.Set(c.Doc("C/b"), testData)
		if errs != nil {
			wr <- nil
			err <- &errs
			return
		}
		wr <- wrs
		err <- &errs
	}
	tcs = append(tcs, testSetCase)

	var testDeleteCase bulkwriterWriteTestCase
	testDeleteCase = func(bw *BulkWriter, wr chan *WriteResult, err chan *error) {
		wrd, errs := bw.Delete(c.Doc("C/c"))
		if errs != nil {
			wr <- nil
			err <- &errs
			return
		}
		wr <- wrd
		err <- &errs
	}
	tcs = append(tcs, testDeleteCase)

	var testUpdateCase bulkwriterWriteTestCase
	testUpdateCase = func(bw *BulkWriter, wr chan *WriteResult, err chan *error) {
		wru, errs := bw.Update(c.Doc("C/f"), []Update{{FieldPath: []string{"*"}, Value: 3}})
		if errs != nil {
			wr <- nil
			err <- &errs
			return
		}
		wr <- wru
		err <- &errs
	}
	tcs = append(tcs, testUpdateCase)

	for i, tc := range tcs {
		wr, err := bulkwriterTestRunner(bw, tc)
		if err != nil {
			t.Errorf("bulkwriter: got error: %v\n", err)
		}
		if wr == nil {
			t.Errorf("bulkwriter: got nil WriteResult and no error")
		}
		if !testEqual(wr, wantWRs[i]) {
			t.Errorf("bulkwriter:\nwanted %v,\n got %v\n", wantWRs[i], wr.UpdateTime)
		}
	}

	if bw.Status().WritesReceivedCount != len(tcs) {
		t.Logf("bulkwriter sent != received; sent: %v, received: %v", len(tcs), bw.Status().WritesReceivedCount)
	}
}
