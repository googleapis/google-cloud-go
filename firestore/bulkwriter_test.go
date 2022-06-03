package firestore

import (
	"sync"
	"testing"
	"time"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

func TestBulkWriter(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	docPrefix := c.Collection("C").Path + "/"
	srv.addRPC(
		&pb.BatchWriteRequest{
			Database: c.path(),
			Writes: []*pb.Write{
				{ // Create
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
				/*
					{ // Set
						Operation: &pb.Write_Update{
							Update: &pb.Document{
								Name:   docPrefix + "b",
								Fields: testFields,
							},
						},
					},
					{ // Delete
						Operation: &pb.Write_Delete{
							Delete: docPrefix + "c",
						},
					},
					{ // Update
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
				*/
			},
		},
		&pb.BatchWriteResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp},
				/*{UpdateTime: aTimestamp2},
				{UpdateTime: aTimestamp3},
				{UpdateTime: aTimestamp3},*/
			},
			Status: []*status.Status{
				{
					Code:    int32(codes.OK),
					Message: "test successful!",
				},
			},
		},
	)

	bw := c.BulkWriter()

	var mu sync.Mutex
	var gotWrites []*pb.WriteResult
	wantWrites := []*pb.WriteResult{
		{UpdateTime: aTimestamp},
		{UpdateTime: aTimestamp2},
		{UpdateTime: aTimestamp3},
		{UpdateTime: aTimestamp3},
	}
	var errs []error

	go func() {
		wrc, err := bw.Create(c.Doc("C/a"), testData)
		if err != nil {
			t.Error("bulkwriter cannot create testData")
			errs = append(errs, err)
			return
		}
		t.Log(wrc)
		mu.Lock()
		defer mu.Unlock()
		gotWrites = append(gotWrites, wrc)
	}()

	/*
		go func() {
			wrs, err := bw.Set(c.Doc("C/b"), testData)
			if err != nil {
				t.Error("bulkwriter cannot set testData")
				errs = append(errs, err)
				return
			}
			t.Log(wrs)
			gotWrites = append(gotWrites, wrs)
		}()

		go func() {
			wru, err := bw.Update(c.Doc("C/f"), []Update{{FieldPath: []string{"*"}, Value: 3}})
			if err != nil {
				t.Error("bulkwriter cannot update testData")
				errs = append(errs, err)
				return
			}
			t.Log(wru)
			gotWrites = append(gotWrites, wru)
		}()

		go func() {
			wrd, err := bw.Delete(c.Doc("C/c"))
			if err != nil {
				t.Error("bulkwriter cannot delete testData")
				errs = append(errs, err)
				return
			}
			t.Log(wrd)
			gotWrites = append(gotWrites, wrd)
		}()

	*/
	for bw.Status().WritesProvidedCount != 1 {
		time.Sleep(time.Duration(time.Millisecond))
	}

	bw.Flush()

	for bw.Status().WritesReceivedCount != 1 {
		time.Sleep(time.Duration(time.Millisecond))
		for _, e := range errs {
			t.Logf("bulkwriter write error: %v", e)
			t.Fatal("bulkwriter encountered too many write errors")
		}
	}

	for i, got := range gotWrites {
		want := wantWrites[i]
		if want != got {
			t.Fatalf("want: %v\ngot:%v", want, got)
		}
	}

	if bw.Status().WritesReceivedCount != 4 {
		t.Logf("bulkwriter sent != received; sent: %v, received: %v", len(wantWrites), bw.Status().WritesReceivedCount)
	}
}
