package firestore

import (
	"sync"
	"testing"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
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
			},
		},
		&pb.BatchWriteResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp},
				{UpdateTime: aTimestamp2},
				{UpdateTime: aTimestamp3},
				{UpdateTime: aTimestamp3},
			},
		},
	)

	bw, err := c.BulkWriter()
	if err != nil {
		t.Error("cannot instantiate bulkwriter")
	}

	wg := sync.WaitGroup{}
	var gotWrites []*pb.WriteResult
	wantWrites := []*pb.WriteResult{
		{UpdateTime: aTimestamp},
		{UpdateTime: aTimestamp2},
		{UpdateTime: aTimestamp3},
		{UpdateTime: aTimestamp3},
	}

	wg.Add(1)
	go func() {
		wrc, err := bw.Create(c.Doc("C/a"), testData)
		if err != nil {
			t.Error("bulkwriter cannot create testData")
		}
		t.Log(wrc)
		gotWrites = append(gotWrites, wrc)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		wrs, err := bw.Set(c.Doc("C/b"), testData)
		if err != nil {
			t.Error("bulkwriter cannot set testData")
		}
		t.Log(wrs)
		gotWrites = append(gotWrites, wrs)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		wru, err := bw.Update(c.Doc("C/f"), []Update{{FieldPath: []string{"*"}, Value: 3}})
		if err != nil {
			t.Error("bulkwriter cannot update testData")
		}
		t.Log(wru)
		gotWrites = append(gotWrites, wru)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		wrd, err := bw.Delete(c.Doc("C/c"))
		if err != nil {
			t.Error("bulkwriter cannot delete testData")
		}
		t.Log(wrd)
		gotWrites = append(gotWrites, wrd)
		wg.Done()
	}()

	for bw.Status().WritesProvidedCount != 4 {
	}

	bw.Flush()
	wg.Wait()

	for i, got := range gotWrites {
		want := wantWrites[i]
		if want != got {
			t.Errorf("want: %v\ngot:%v", want, got)
		}
	}
}
