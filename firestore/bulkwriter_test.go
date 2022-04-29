package firestore

import "testing"

func TestBulkWriter(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	docPrefix := c.Collection("C").Path + "/"
	srv.addRPC(
		&pb.CommitRequest{
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
		&pb.CommitResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp},
				{UpdateTime: aTimestamp2},
				{UpdateTime: aTimestamp3},
			},
		},
	)
	gotWRs, err := c.BulkWriter().
		Create(c.Doc("C/a"), testData).
		Set(c.Doc("C/b"), testData).
		Delete(c.Doc("C/c")).
		Update(c.Doc("C/f"), []Update{{FieldPath: []string{"*"}, Value: 3}}).
		Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantWRs := []*WriteResult{{aTime}, {aTime2}, {aTime3}}
	if !testEqual(gotWRs, wantWRs) {
		t.Errorf("got  %+v\nwant %+v", gotWRs, wantWRs)
	}
}

func TestBulkWriterErrors(t *testing.T) {
	ctx := context.Background()
	c, _, cleanup := newMock(t)
	defer cleanup()

	for _, test := range []struct {
		desc string
		bw   *BulkWriter
	}{
		{
			"empty batch",
			c.BulkWriter(),
		},
		{
			"bad doc reference",
			c.BulkWriter().Create(c.Doc("a"), testData),
		},
		{
			"bad data",
			c.BulkWriter().Create(c.Doc("a/b"), 3),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			if _, err := test.bw.Close(); err == nil {
				t.Fatal("got nil, want error")
			}
		})
	}
}
