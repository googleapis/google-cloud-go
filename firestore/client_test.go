// Copyright 2017 Google LLC
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
	"os"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

var testClient = &Client{
	projectID:    "projectID",
	databaseID:   "(default)",
	readSettings: &readSettings{},
}

var testClientNamedDB = &Client{
	projectID:    "projectID",
	databaseID:   "my-database",
	readSettings: &readSettings{},
}

func TestNewClientWithDatabase(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		databaseID string
		projectID  string
		wantErr    bool
	}{
		{
			desc:       "Empty databaseID",
			databaseID: "",
			projectID:  "p1",
			wantErr:    true,
		},
		{
			desc:       "Error from NewClient bubbled to NewClientWithDatabase",
			databaseID: "db1",
			projectID:  "",
			wantErr:    true,
		},
		{
			desc:       "Valid databaseID",
			databaseID: "db1",
			projectID:  "p1",
			wantErr:    false,
		},
	} {
		client, err := NewClientWithDatabase(context.Background(), tc.projectID, tc.databaseID)

		if err != nil && !tc.wantErr {
			t.Errorf("NewClientWithDatabase: %s got %v want nil", tc.desc, err)
		} else if err == nil && tc.wantErr {
			t.Errorf("NewClientWithDatabase: %s got %v wanted error", tc.desc, err)
		} else if err == nil && tc.databaseID != client.databaseID {
			t.Errorf("NewClientWithDatabase: %s got %v want %v", tc.desc, client.databaseID, tc.databaseID)
		}

	}
}

func TestClientCollectionAndDoc(t *testing.T) {
	coll1 := testClient.Collection("X")
	db := "projects/projectID/databases/(default)"
	wantc1 := &CollectionRef{
		c:          testClient,
		parentPath: db + "/documents",
		selfPath:   "X",
		Parent:     nil,
		ID:         "X",
		Path:       "projects/projectID/databases/(default)/documents/X",
		Query: Query{
			c:            testClient,
			collectionID: "X",
			path:         "projects/projectID/databases/(default)/documents/X",
			parentPath:   db + "/documents",
		},
		readSettings: &readSettings{},
	}
	if !testEqual(coll1, wantc1) {
		t.Fatalf("got\n%+v\nwant\n%+v", coll1, wantc1)
	}
	doc1 := testClient.Doc("X/a")
	wantd1 := &DocumentRef{
		Parent:       coll1,
		ID:           "a",
		Path:         "projects/projectID/databases/(default)/documents/X/a",
		shortPath:    "X/a",
		readSettings: &readSettings{},
	}

	if !testEqual(doc1, wantd1) {
		t.Fatalf("got %+v, want %+v", doc1, wantd1)
	}
	coll2 := testClient.Collection("X/a/Y")
	parentPath := "projects/projectID/databases/(default)/documents/X/a"
	wantc2 := &CollectionRef{
		c:          testClient,
		parentPath: parentPath,
		selfPath:   "X/a/Y",
		Parent:     doc1,
		ID:         "Y",
		Path:       "projects/projectID/databases/(default)/documents/X/a/Y",
		Query: Query{
			c:            testClient,
			collectionID: "Y",
			parentPath:   parentPath,
			path:         "projects/projectID/databases/(default)/documents/X/a/Y",
		},
	}
	if !testEqual(coll2, wantc2) {
		t.Fatalf("\ngot  %+v\nwant %+v", coll2, wantc2)
	}
	doc2 := testClient.Doc("X/a/Y/b")
	wantd2 := &DocumentRef{
		Parent:    coll2,
		ID:        "b",
		Path:      "projects/projectID/databases/(default)/documents/X/a/Y/b",
		shortPath: "X/a/Y/b",
	}
	if !testEqual(doc2, wantd2) {
		t.Fatalf("got %+v, want %+v", doc2, wantd2)
	}
}

func TestClientCollDocErrors(t *testing.T) {
	for _, badColl := range []string{"", "/", "/a/", "/a/b", "a/b/", "a//b"} {
		coll := testClient.Collection(badColl)
		if coll != nil {
			t.Errorf("coll path %q: got %+v, want nil", badColl, coll)
		}
	}
	for _, badDoc := range []string{"", "a", "/", "/a", "a/", "a/b/c", "a//b/c"} {
		doc := testClient.Doc(badDoc)
		if doc != nil {
			t.Errorf("doc path %q: got %+v, want nil", badDoc, doc)
		}
	}
}

func TestClientDocFromResourceName(t *testing.T) {
	defaultDBPath := "projects/projectID/databases/(default)"
	namedDBPath := "projects/projectID/databases/my-database"

	myCollectionRef := &CollectionRef{
		c:          testClient,
		parentPath: defaultDBPath + "/documents",
		selfPath:   "myCollection",
		ID:         "myCollection",
		Path:       defaultDBPath + "/documents/myCollection",
		Query: Query{
			c:            testClient,
			collectionID: "myCollection",
			path:         defaultDBPath + "/documents/myCollection",
			parentPath:   defaultDBPath + "/documents",
		},
		readSettings: &readSettings{},
	}
	myDocRef := &DocumentRef{
		Parent:       myCollectionRef,
		ID:           "myDoc",
		Path:         defaultDBPath + "/documents/myCollection/myDoc",
		shortPath:    "myCollection/myDoc",
		readSettings: &readSettings{},
	}
	subCollectionRef := &CollectionRef{
		c:          testClient,
		parentPath: defaultDBPath + "/documents/myCollection/myDoc",
		selfPath:   "myCollection/myDoc/subCollection",
		Parent:     myDocRef,
		ID:         "subCollection",
		Path:       defaultDBPath + "/documents/myCollection/myDoc/subCollection",
		Query: Query{
			c:            testClient,
			collectionID: "subCollection",
			path:         defaultDBPath + "/documents/myCollection/myDoc/subCollection",
			parentPath:   defaultDBPath + "/documents/myCollection/myDoc",
		},
		readSettings: &readSettings{},
	}

	tests := []struct {
		name         string
		client       *Client
		resourceName string
		want         *DocumentRef
	}{
		{
			name:         "Valid resource name for default database",
			client:       testClient,
			resourceName: defaultDBPath + "/documents/myCollection/myDoc",
			want: &DocumentRef{
				Parent:       myCollectionRef, // Use the pre-constructed parent
				ID:           "myDoc",
				Path:         defaultDBPath + "/documents/myCollection/myDoc",
				shortPath:    "myCollection/myDoc",
				readSettings: &readSettings{},
			},
		},
		{
			name:         "Valid resource name for named database",
			client:       testClientNamedDB,
			resourceName: namedDBPath + "/documents/users/user1",
			want: &DocumentRef{
				Parent: &CollectionRef{
					c:          testClientNamedDB,
					parentPath: namedDBPath + "/documents",
					selfPath:   "users",
					ID:         "users",
					Path:       namedDBPath + "/documents/users",
					Query: Query{
						c:            testClientNamedDB,
						collectionID: "users",
						path:         namedDBPath + "/documents/users",
						parentPath:   namedDBPath + "/documents",
					},
					readSettings: &readSettings{},
				},
				ID:           "user1",
				Path:         namedDBPath + "/documents/users/user1",
				shortPath:    "users/user1",
				readSettings: &readSettings{},
			},
		},
		{
			name:         "Valid resource name with subcollection",
			client:       testClient,
			resourceName: defaultDBPath + "/documents/myCollection/myDoc/subCollection/subDoc",
			want: &DocumentRef{
				Parent:       subCollectionRef, // Use the pre-constructed parent
				ID:           "subDoc",
				Path:         defaultDBPath + "/documents/myCollection/myDoc/subCollection/subDoc",
				shortPath:    "myCollection/myDoc/subCollection/subDoc",
				readSettings: &readSettings{},
			},
		},
		{
			name:         "Empty resource name",
			client:       testClient,
			resourceName: "",
			want:         nil,
		},
		{
			name:         "Resource name missing 'projects/' prefix",
			client:       testClient,
			resourceName: "myCollection/myDoc", // Not a full resource name
			want:         nil,
		},
		{
			name:         "Resource name missing '/documents/' segment",
			client:       testClient,
			resourceName: defaultDBPath + "/myCollection/myDoc",
			want:         nil,
		},
		{
			name:         "Resource name with malformed '/documents/' split",
			client:       testClient,
			resourceName: defaultDBPath + "/documents", // ends with /documents
			want:         nil,
		},
		{
			name:         "Resource name refers to a collection (odd segments after /documents/)",
			client:       testClient,
			resourceName: defaultDBPath + "/documents/myCollection",
			want:         nil,
		},
		{
			name:         "Resource name with mismatched project ID",
			client:       testClient,
			resourceName: "projects/otherProject/databases/(default)/documents/myCollection/myDoc",
			want:         nil,
		},
		{
			name:         "Resource name with mismatched database ID",
			client:       testClient,
			resourceName: "projects/projectID/databases/otherDB/documents/myCollection/myDoc",
			want:         nil,
		},
		{
			name:         "Resource name with empty segment after /documents/",
			client:       testClient,
			resourceName: defaultDBPath + "/documents/myCollection//myDoc",
			want:         nil,
		},
		{
			name:         "Resource name with empty segment in collection part",
			client:       testClient,
			resourceName: defaultDBPath + "/documents//myDoc",
			want:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.client.DocFromFullPath(tt.resourceName)
			if !testEqual(got, tt.want) {
				t.Errorf("DocFromResourceName(%q) got %+v, want %+v", tt.resourceName, got, tt.want)
			}
		})
	}
}

func TestGetAll(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const dbPath = "projects/projectID/databases/(default)"
	req := &pb.BatchGetDocumentsRequest{
		Database: dbPath,
		Documents: []string{
			dbPath + "/documents/C/a",
			dbPath + "/documents/C/b",
			dbPath + "/documents/C/c",
		},
	}
	testGetAll(t, c, srv, dbPath, func(drs []*DocumentRef) ([]*DocumentSnapshot, error) {
		return c.GetAll(context.Background(), drs)
	}, req)
}

func testGetAll(t *testing.T, c *Client, srv *mockServer, dbPath string, getAll func([]*DocumentRef) ([]*DocumentSnapshot, error), req *pb.BatchGetDocumentsRequest) {
	wantPBDocs := []*pb.Document{
		{
			Name:       dbPath + "/documents/C/a",
			CreateTime: aTimestamp,
			UpdateTime: aTimestamp,
			Fields:     map[string]*pb.Value{"f": intval(2)},
		},
		nil,
		{
			Name:       dbPath + "/documents/C/c",
			CreateTime: aTimestamp,
			UpdateTime: aTimestamp,
			Fields:     map[string]*pb.Value{"f": intval(1)},
		},
	}
	wantReadTimes := []*tspb.Timestamp{aTimestamp, aTimestamp2, aTimestamp3}
	srv.addRPC(req,
		[]interface{}{
			// deliberately put these out of order
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: wantPBDocs[2]},
				ReadTime: aTimestamp3,
			},
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: wantPBDocs[0]},
				ReadTime: aTimestamp,
			},
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Missing{Missing: dbPath + "/documents/C/b"},
				ReadTime: aTimestamp2,
			},
		},
	)
	coll := c.Collection("C")
	var docRefs []*DocumentRef
	for _, name := range []string{"a", "b", "c"} {
		docRefs = append(docRefs, coll.Doc(name))
	}
	docs, err := getAll(docRefs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(docs), len(wantPBDocs); got != want {
		t.Errorf("got %d docs, wanted %d", got, want)
	}
	for i, got := range docs {
		want, err := newDocumentSnapshot(docRefs[i], wantPBDocs[i], c, wantReadTimes[i])
		if err != nil {
			t.Fatal(err)
		}
		if diff := testDiff(got, want); diff != "" {
			t.Errorf("#%d: got=--, want==++\n%s", i, diff)
		}
	}
}

func TestGetAllWithEqualRefs(t *testing.T) {
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const dbPath = "projects/projectID/databases/(default)"
	req := &pb.BatchGetDocumentsRequest{
		Database: dbPath,
		Documents: []string{
			dbPath + "/documents/C/a",
			dbPath + "/documents/C/a",
			dbPath + "/documents/C/c",
			dbPath + "/documents/C/a",
			dbPath + "/documents/C/b",
			dbPath + "/documents/C/c",
			dbPath + "/documents/C/b",
		},
	}
	testGetAllWithEqualRefs(t, c, srv, dbPath, func(drs []*DocumentRef) ([]*DocumentSnapshot, error) {
		return c.GetAll(context.Background(), drs)
	}, req)
}

func testGetAllWithEqualRefs(t *testing.T, c *Client, srv *mockServer, dbPath string, getAll func([]*DocumentRef) ([]*DocumentSnapshot, error), req *pb.BatchGetDocumentsRequest) {
	wantPBDocs := []*pb.Document{
		{
			Name:       dbPath + "/documents/C/a",
			CreateTime: aTimestamp,
			UpdateTime: aTimestamp,
			Fields:     map[string]*pb.Value{"f": intval(2)},
		},
		{
			Name:       dbPath + "/documents/C/c",
			CreateTime: aTimestamp,
			UpdateTime: aTimestamp,
			Fields:     map[string]*pb.Value{"f": intval(1)},
		},
		nil,
	}
	srv.addRPC(req,
		[]interface{}{
			// deliberately put these out of order
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: wantPBDocs[1]},
				ReadTime: aTimestamp3,
			},
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: wantPBDocs[0]},
				ReadTime: aTimestamp,
			},
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Missing{Missing: dbPath + "/documents/C/b"},
				ReadTime: aTimestamp2,
			},
		},
	)
	coll := c.Collection("C")
	var docRefs []*DocumentRef
	for _, name := range []string{"a", "a", "c", "a", "b", "c", "b"} {
		docRefs = append(docRefs, coll.Doc(name))
	}
	// GetAll should return the same number of document snapshots as the
	// number of document references in the input range, even when that means
	// that the same document snapshot is referenced multiple times in the
	// returned collection.
	docs, err := getAll(docRefs)
	if err != nil {
		t.Fatal(err)
	}
	wantDocsIndices := []int{0, 0, 1, 0, 2, 1, 2}
	wantReadTimes := []*tspb.Timestamp{aTimestamp, aTimestamp, aTimestamp3, aTimestamp, aTimestamp2, aTimestamp3, aTimestamp2}
	if got, want := len(docs), len(wantDocsIndices); got != want {
		t.Errorf("got %d docs, wanted %d", got, want)
	}
	for i, got := range docs {
		want, err := newDocumentSnapshot(docRefs[i], wantPBDocs[wantDocsIndices[i]], c, wantReadTimes[i])
		if err != nil {
			t.Fatal(err)
		}
		if diff := testDiff(got, want); diff != "" {
			t.Errorf("#%d: got=--, want==++\n%s", i, diff)
		}
	}
}

func TestGetAllErrors(t *testing.T) {
	ctx := context.Background()
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const dbPath = "projects/projectID/databases/(default)"
	const docPath = dbPath + "/documents/C/a"
	if _, err := c.GetAll(ctx, []*DocumentRef{nil}); err != errNilDocRef {
		t.Errorf("got %v, want errNilDocRef", err)
	}

	// Internal server error.
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:  dbPath,
			Documents: []string{docPath},
		},
		[]interface{}{status.Errorf(codes.Internal, "")},
	)
	_, err := c.GetAll(ctx, []*DocumentRef{c.Doc("C/a")})
	codeEq(t, "GetAll #1", codes.Internal, err)

	// Doc appears as both found and missing (server bug).
	srv.reset()
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:  dbPath,
			Documents: []string{docPath},
		},
		[]interface{}{
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{Found: &pb.Document{Name: docPath}},
				ReadTime: aTimestamp,
			},
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Missing{Missing: docPath},
				ReadTime: aTimestamp,
			},
		},
	)
	if _, err := c.GetAll(ctx, []*DocumentRef{c.Doc("C/a")}); err == nil {
		t.Error("got nil, want error")
	}

	// Invalid UTF-8 characters
	srv.reset()
	if _, gotErr := c.GetAll(ctx, []*DocumentRef{c.Doc("C/Mayag\xcfez")}); !errorsMatch(gotErr, errInvalidUtf8DocRef) {
		t.Errorf("got: %v, want: %v", gotErr, errInvalidUtf8DocRef)
	}
}

func TestClient_WithReadOptions(t *testing.T) {
	ctx := context.Background()
	c, srv, cleanup := newMock(t)
	defer cleanup()

	const dbPath = "projects/projectID/databases/(default)"
	const docPath = dbPath + "/documents/C/a"
	tm := time.Date(2021, time.February, 20, 0, 0, 0, 0, time.UTC)

	dr := &DocumentRef{
		Parent: &CollectionRef{
			c: c,
		},
		ID:   "123",
		Path: docPath,
	}

	srv.addRPC(&pb.BatchGetDocumentsRequest{
		Database:  dbPath,
		Documents: []string{docPath},
		ConsistencySelector: &pb.BatchGetDocumentsRequest_ReadTime{
			ReadTime: &tspb.Timestamp{Seconds: tm.Unix()},
		},
	}, []interface{}{
		&pb.BatchGetDocumentsResponse{
			ReadTime: &tspb.Timestamp{Seconds: tm.Unix()},
			Result: &pb.BatchGetDocumentsResponse_Found{
				Found: &pb.Document{},
			},
		},
	})

	_, err := c.WithReadOptions(ReadTime(tm)).GetAll(ctx, []*DocumentRef{
		dr,
	})

	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_UsesEmulator(t *testing.T) {
	c, _, cleanup := newMock(t)
	defer cleanup()
	if c.UsesEmulator {
		t.Error("got true, want false")
	}

	os.Setenv("FIRESTORE_EMULATOR_HOST", "localhost:8080")
	defer os.Unsetenv("FIRESTORE_EMULATOR_HOST")
	c, _, cleanup = newMock(t)
	defer cleanup()
	if !c.UsesEmulator {
		t.Error("got false, want true")
	}
}
