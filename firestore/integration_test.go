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
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	apiv1 "cloud.google.com/go/firestore/apiv1/admin"
	"cloud.google.com/go/firestore/apiv1/admin/adminpb"
	firestorev1 "cloud.google.com/go/firestore/apiv1/firestorepb"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestMain(m *testing.M) {
	databaseIDs := []string{DefaultDatabaseID}
	databasesStr, ok := os.LookupEnv(envDatabases)
	if ok {
		databaseIDs = append(databaseIDs, strings.Split(databasesStr, ",")...)
	}

	testParams = make(map[string]interface{})
	for _, databaseID := range databaseIDs {
		testParams["databaseID"] = databaseID
		initIntegrationTest()
		status := m.Run()
		if status != 0 {
			os.Exit(status)
		}
		cleanupIntegrationTest()
	}

	os.Exit(0)
}

const (
	envProjID     = "GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID"
	envPrivateKey = "GCLOUD_TESTS_GOLANG_FIRESTORE_KEY"
	envDatabases  = "GCLOUD_TESTS_GOLANG_FIRESTORE_DATABASES"
)

var (
	iClient       *Client
	iAdminClient  *apiv1.FirestoreAdminClient
	iColl         *CollectionRef
	collectionIDs = uid.NewSpace("go-integration-test", nil)
	wantDBPath    string
	indexNames    []string
	testParams    map[string]interface{}
)

func initIntegrationTest() {
	databaseID := testParams["databaseID"].(string)
	log.Printf("Setting up tests to run on databaseID: %q\n", databaseID)
	flag.Parse() // needed for testing.Short()
	if testing.Short() {
		return
	}
	ctx := context.Background()
	testProjectID := os.Getenv(envProjID)
	if testProjectID == "" {
		log.Println("Integration tests skipped. See CONTRIBUTING.md for details")
		return
	}
	ts := testutil.TokenSourceEnv(ctx, envPrivateKey,
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/datastore")
	if ts == nil {
		log.Fatal("The project key must be set. See CONTRIBUTING.md for details")
	}
	projectPath := "projects/" + testProjectID
	wantDBPath = projectPath + "/databases/" + databaseID

	ti := &testutil.HeadersEnforcer{
		Checkers: []*testutil.HeaderChecker{
			testutil.XGoogClientHeaderChecker,

			{
				Key: "google-cloud-resource-prefix",
				ValuesValidator: func(values ...string) error {
					if len(values) == 0 {
						return errors.New("expected non-blank header")
					}
					if values[0] != wantDBPath {
						return fmt.Errorf("resource prefix mismatch; got %q want %q", values[0], wantDBPath)
					}
					return nil
				},
			},
		},
	}
	copts := append(ti.CallOptions(), option.WithTokenSource(ts))
	c, err := NewClientWithDatabase(ctx, testProjectID, databaseID, copts...)
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	iClient = c
	iColl = c.Collection(collectionIDs.New())

	adminC, err := apiv1.NewFirestoreAdminClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("NewFirestoreAdminClient: %v", err)
	}
	iAdminClient = adminC

	createIndexes(ctx, wantDBPath)

	refDoc := iColl.NewDoc()
	integrationTestMap["ref"] = refDoc
	wantIntegrationTestMap["ref"] = refDoc
	integrationTestStruct.Ref = refDoc
}

// createIndexes creates composite indexes on provided Firestore database
// Indexes are required to run queries with composite filters on multiple fields.
// Without indexes, FailedPrecondition rpc error is seen with
// desc 'The query requires multiple indexes'.
func createIndexes(ctx context.Context, dbPath string) {

	indexFields := [][]string{
		{"updatedAt", "weight", "height"},
		{"weight", "height"},
		{"width", "depth"},
		{"width", "model"}}
	indexNames = make([]string, len(indexFields))
	indexParent := fmt.Sprintf("%s/collectionGroups/%s", dbPath, iColl.ID)

	var wg sync.WaitGroup
	for i, fields := range indexFields {
		wg.Add(1)
		var adminPbIndexFields []*adminpb.Index_IndexField
		for _, field := range fields {
			adminPbIndexFields = append(adminPbIndexFields, &adminpb.Index_IndexField{
				FieldPath: field,
				ValueMode: &adminpb.Index_IndexField_Order_{
					Order: adminpb.Index_IndexField_ASCENDING,
				},
			})
		}
		req := &adminpb.CreateIndexRequest{
			Parent: indexParent,
			Index: &adminpb.Index{
				QueryScope: adminpb.Index_COLLECTION,
				Fields:     adminPbIndexFields,
			},
		}
		op, createErr := iAdminClient.CreateIndex(ctx, req)
		if createErr != nil {
			log.Fatalf("CreateIndex: %v", createErr)
		}
		if i == 0 {
			// Seed first index to prevent FirestoreMetadataWrite.BootstrapDatabase Concurrent access error
			handleCreateIndexResp(ctx, &wg, i, op)
		} else {
			go handleCreateIndexResp(ctx, &wg, i, op)
		}
	}
	wg.Wait()
}

func handleCreateIndexResp(ctx context.Context, wg *sync.WaitGroup, i int, op *apiv1.CreateIndexOperation) {
	defer wg.Done()
	createdIndex, waitErr := op.Wait(ctx)
	if waitErr != nil {
		log.Fatalf("Wait: %v", waitErr)
	}
	indexNames[i] = createdIndex.Name
}

// deleteIndexes deletes composite indexes created in createIndexes function
func deleteIndexes(ctx context.Context) {
	for _, indexName := range indexNames {
		err := iAdminClient.DeleteIndex(ctx, &adminpb.DeleteIndexRequest{
			Name: indexName,
		})
		if err != nil {
			log.Printf("Failed to delete index \"%s\": %+v\n", indexName, err)
		}
	}
}

// deleteCollection recursively deletes the documents in the specified collection
func deleteCollection(ctx context.Context, coll *CollectionRef) error {
	bulkwriter := iClient.BulkWriter(ctx)

	// Get  documents
	iter := coll.Documents(ctx)

	// Iterate through the documents, adding
	// a delete operation for each one to the BulkWriter.
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to get next document: %+v\n", err)
			return err
		}

		err = deleteDocument(ctx, doc.Ref, bulkwriter)
		if err != nil {
			log.Printf("Failed to delete document: %+v\n", err)
			return err
		}
	}

	bulkwriter.End()
	bulkwriter.Flush()

	return nil
}

func deleteDocuments(docRefs []*DocumentRef) {
	if testing.Short() {
		return
	}
	ctx := context.Background()
	bulkwriter := iClient.BulkWriter(ctx)
	for _, docRef := range docRefs {
		if err := deleteDocument(ctx, docRef, bulkwriter); err != nil {
			log.Printf("Failed to delete document: %s with error %+v", docRef.ID, err)
		}
	}
	bulkwriter.End()
	bulkwriter.Flush()
}

func deleteDocumentSnapshots(docSnaps []*DocumentSnapshot) {
	var docRefs []*DocumentRef
	for _, docSnap := range docSnaps {
		docRefs = append(docRefs, docSnap.Ref)
	}
	deleteDocuments(docRefs)
}

func deleteDocument(ctx context.Context, docRef *DocumentRef, bulkwriter *BulkWriter) error {
	// Delete subcollections before deleting document
	subCollIter := docRef.Collections(ctx)
	for {
		subColl, err := subCollIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Error getting next subcollection of %s: %+v", docRef.ID, err)
			return err
		}
		err = deleteCollection(ctx, subColl)
		if err != nil {
			log.Printf("Error deleting subcollection %v: %+v\n", subColl.ID, err)
			return err
		}
	}

	// Delete document
	_, err := bulkwriter.Delete(docRef)
	if err != nil {
		log.Printf("Failed to delete document: %+v, err: %+v\n", docRef, err)
		return err
	}
	return nil
}

func cleanupIntegrationTest() {
	if iClient != nil {
		ctx := context.Background()
		deleteIndexes(ctx)
		deleteCollection(ctx, iColl)
		iClient.Close()
	}

	if iAdminClient != nil {
		iAdminClient.Close()
	}
}

// integrationClient should be called by integration tests to get a valid client. It will never
// return nil. If integrationClient returns, an integration test can proceed without
// further checks.
func integrationClient(t *testing.T) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	if iClient == nil {
		t.SkipNow() // log message printed in initIntegrationTest
	}
	return iClient
}

func integrationColl(t *testing.T) *CollectionRef {
	_ = integrationClient(t)
	return iColl
}

type integrationTestStructType struct {
	Int         int
	Str         string
	Bool        bool
	Float       float32
	Null        interface{}
	Bytes       []byte
	Time        time.Time
	Geo, NilGeo *latlng.LatLng
	Ref         *DocumentRef
}

var (
	integrationTime = time.Date(2017, 3, 20, 1, 2, 3, 456789, time.UTC)
	// Firestore times are accurate only to microseconds.
	wantIntegrationTime = time.Date(2017, 3, 20, 1, 2, 3, 456000, time.UTC)

	integrationGeo = &latlng.LatLng{Latitude: 30, Longitude: 70}

	// Use this when writing a doc.
	integrationTestMap = map[string]interface{}{
		"int":    1,
		"int8":   int8(2),
		"int16":  int16(3),
		"int32":  int32(4),
		"int64":  int64(5),
		"uint8":  uint8(6),
		"uint16": uint16(7),
		"uint32": uint32(8),
		"str":    "two",
		"bool":   true,
		"float":  3.14,
		"null":   nil,
		"bytes":  []byte("bytes"),
		"*":      map[string]interface{}{"`": 4},
		"time":   integrationTime,
		"geo":    integrationGeo,
		"ref":    nil, // populated by initIntegrationTest
	}

	// The returned data is slightly different.
	wantIntegrationTestMap = map[string]interface{}{
		"int":    int64(1),
		"int8":   int64(2),
		"int16":  int64(3),
		"int32":  int64(4),
		"int64":  int64(5),
		"uint8":  int64(6),
		"uint16": int64(7),
		"uint32": int64(8),
		"str":    "two",
		"bool":   true,
		"float":  3.14,
		"null":   nil,
		"bytes":  []byte("bytes"),
		"*":      map[string]interface{}{"`": int64(4)},
		"time":   wantIntegrationTime,
		"geo":    integrationGeo,
		"ref":    nil, // populated by initIntegrationTest
	}

	integrationTestStruct = integrationTestStructType{
		Int:    1,
		Str:    "two",
		Bool:   true,
		Float:  3.14,
		Null:   nil,
		Bytes:  []byte("bytes"),
		Time:   integrationTime,
		Geo:    integrationGeo,
		NilGeo: nil,
		Ref:    nil, // populated by initIntegrationTest
	}
)

func TestIntegration_Create(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	start := time.Now()
	h := testHelper{t}
	wr := h.mustCreate(doc, integrationTestMap)
	end := time.Now()
	checkTimeBetween(t, wr.UpdateTime, start, end)
	_, err := doc.Create(ctx, integrationTestMap)
	codeEq(t, "Create on a present doc", codes.AlreadyExists, err)
	// OK to create an empty document.
	emptyDoc := integrationColl(t).NewDoc()
	_, err = emptyDoc.Create(ctx, map[string]interface{}{})
	codeEq(t, "Create empty doc", codes.OK, err)
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc, emptyDoc})
	})
}

func TestIntegration_Get(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	h.mustCreate(doc, integrationTestMap)
	ds := h.mustGet(doc)
	if ds.CreateTime != ds.UpdateTime {
		t.Errorf("create time %s != update time %s", ds.CreateTime, ds.UpdateTime)
	}
	got := ds.Data()
	if want := wantIntegrationTestMap; !testEqual(got, want) {
		t.Errorf("got\n%v\nwant\n%v", pretty.Value(got), pretty.Value(want))
	}

	emptyDoc := integrationColl(t).NewDoc()
	empty := map[string]interface{}{}
	h.mustCreate(emptyDoc, empty)
	ds = h.mustGet(emptyDoc)
	if ds.CreateTime != ds.UpdateTime {
		t.Errorf("create time %s != update time %s", ds.CreateTime, ds.UpdateTime)
	}
	if got, want := ds.Data(), empty; !testEqual(got, want) {
		t.Errorf("got\n%v\nwant\n%v", pretty.Value(got), pretty.Value(want))
	}

	ds, err := integrationColl(t).NewDoc().Get(ctx)
	codeEq(t, "Get on a missing doc", codes.NotFound, err)
	if ds == nil || ds.Exists() {
		t.Error("got nil or existing doc snapshot, want !ds.Exists")
	}
	if ds.ReadTime.IsZero() {
		t.Error("got zero read time")
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc, emptyDoc})
	})
}

func TestIntegration_GetAll(t *testing.T) {
	type getAll struct{ N int }

	h := testHelper{t}
	coll := integrationColl(t)
	ctx := context.Background()
	var docRefs []*DocumentRef
	for i := 0; i < 5; i++ {
		doc := coll.NewDoc()
		docRefs = append(docRefs, doc)
		if i != 3 {
			h.mustCreate(doc, getAll{N: i})
		}
	}
	docSnapshots, err := iClient.GetAll(ctx, docRefs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(docSnapshots), len(docRefs); got != want {
		t.Fatalf("got %d snapshots, want %d", got, want)
	}
	for i, ds := range docSnapshots {
		if i == 3 {
			if ds == nil || ds.Exists() {
				t.Fatal("got nil or existing doc snapshot, want !ds.Exists")
			}
			err := ds.DataTo(nil)
			codeEq(t, "DataTo on a missing doc", codes.NotFound, err)
		} else {
			var got getAll
			if err := ds.DataTo(&got); err != nil {
				t.Fatal(err)
			}
			want := getAll{N: i}
			if got != want {
				t.Errorf("%d: got %+v, want %+v", i, got, want)
			}
		}
		if ds.ReadTime.IsZero() {
			t.Errorf("%d: got zero read time", i)
		}
	}
	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})
}

func TestIntegration_Add(t *testing.T) {
	start := time.Now()
	docRef, wr, err := integrationColl(t).Add(context.Background(), integrationTestMap)
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now()
	checkTimeBetween(t, wr.UpdateTime, start, end)
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{docRef})
	})
}

func TestIntegration_Set(t *testing.T) {
	coll := integrationColl(t)
	h := testHelper{t}
	ctx := context.Background()

	// Set Should be able to create a new doc.
	doc := coll.NewDoc()
	wr1 := h.mustSet(doc, integrationTestMap)
	// Calling Set on the doc completely replaces the contents.
	// The update time should increase.
	newData := map[string]interface{}{
		"str": "change",
		"x":   "1",
	}
	wr2 := h.mustSet(doc, newData)
	if !wr1.UpdateTime.Before(wr2.UpdateTime) {
		t.Errorf("update time did not increase: old=%s, new=%s", wr1.UpdateTime, wr2.UpdateTime)
	}
	ds := h.mustGet(doc)
	if got := ds.Data(); !testEqual(got, newData) {
		t.Errorf("got %v, want %v", got, newData)
	}

	newData = map[string]interface{}{
		"str": "1",
		"x":   "2",
		"y":   "3",
	}
	// SetOptions:
	// Only fields mentioned in the Merge option will be changed.
	// In this case, "str" will not be changed to "1".
	wr3, err := doc.Set(ctx, newData, Merge([]string{"x"}, []string{"y"}))
	if err != nil {
		t.Fatal(err)
	}
	ds = h.mustGet(doc)
	want := map[string]interface{}{
		"str": "change",
		"x":   "2",
		"y":   "3",
	}
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !wr2.UpdateTime.Before(wr3.UpdateTime) {
		t.Errorf("update time did not increase: old=%s, new=%s", wr2.UpdateTime, wr3.UpdateTime)
	}

	// Another way to change only x and y is to pass a map with only
	// those keys, and use MergeAll.
	wr4, err := doc.Set(ctx, map[string]interface{}{"x": "4", "y": "5"}, MergeAll)
	if err != nil {
		t.Fatal(err)
	}
	ds = h.mustGet(doc)
	want = map[string]interface{}{
		"str": "change",
		"x":   "4",
		"y":   "5",
	}
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !wr3.UpdateTime.Before(wr4.UpdateTime) {
		t.Errorf("update time did not increase: old=%s, new=%s", wr3.UpdateTime, wr4.UpdateTime)
	}

	// use firestore.Delete to delete a field.
	// TODO(deklerk): We should be able to use mustSet, but then we get a test error. We should investigate this.
	_, err = doc.Set(ctx, map[string]interface{}{"str": Delete}, MergeAll)
	if err != nil {
		t.Fatal(err)
	}
	ds = h.mustGet(doc)
	want = map[string]interface{}{
		"x": "4",
		"y": "5",
	}
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Writing an empty doc with MergeAll should create the doc.
	doc2 := coll.NewDoc()
	want = map[string]interface{}{}
	h.mustSet(doc2, want, MergeAll)
	ds = h.mustGet(doc2)
	if got := ds.Data(); !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc, doc2})
	})
}

func TestIntegration_Delete(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	h.mustCreate(doc, integrationTestMap)
	h.mustDelete(doc)
	// Confirm that doc doesn't exist.
	if _, err := doc.Get(ctx); status.Code(err) != codes.NotFound {
		t.Fatalf("got error <%v>, want NotFound", err)
	}

	er := func(_ *WriteResult, err error) error { return err }

	codeEq(t, "Delete on a missing doc", codes.OK,
		er(doc.Delete(ctx)))
	// TODO(jba): confirm that the server should return InvalidArgument instead of
	// FailedPrecondition.
	wr := h.mustCreate(doc, integrationTestMap)
	codeEq(t, "Delete with wrong LastUpdateTime", codes.FailedPrecondition,
		er(doc.Delete(ctx, LastUpdateTime(wr.UpdateTime.Add(-time.Millisecond)))))
	codeEq(t, "Delete with right LastUpdateTime", codes.OK,
		er(doc.Delete(ctx, LastUpdateTime(wr.UpdateTime))))
}

func TestIntegration_Update(t *testing.T) {
	ctx := context.Background()
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}

	h.mustCreate(doc, integrationTestMap)
	fpus := []Update{
		{Path: "bool", Value: false},
		{Path: "time", Value: 17},
		{FieldPath: []string{"*", "`"}, Value: 18},
		{Path: "null", Value: Delete},
		{Path: "noSuchField", Value: Delete}, // deleting a non-existent field is a no-op
	}
	wr := h.mustUpdate(doc, fpus)
	ds := h.mustGet(doc)
	got := ds.Data()
	want := copyMap(wantIntegrationTestMap)
	want["bool"] = false
	want["time"] = int64(17)
	want["*"] = map[string]interface{}{"`": int64(18)}
	delete(want, "null")
	if !testEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}

	er := func(_ *WriteResult, err error) error { return err }

	codeEq(t, "Update on missing doc", codes.NotFound,
		er(integrationColl(t).NewDoc().Update(ctx, fpus)))
	codeEq(t, "Update with wrong LastUpdateTime", codes.FailedPrecondition,
		er(doc.Update(ctx, fpus, LastUpdateTime(wr.UpdateTime.Add(-time.Millisecond)))))
	codeEq(t, "Update with right LastUpdateTime", codes.OK,
		er(doc.Update(ctx, fpus, LastUpdateTime(wr.UpdateTime))))

	// Verify that map value deletion is respected
	fpus = []Update{
		{FieldPath: []string{"*", "`"}, Value: Delete},
	}
	_ = h.mustUpdate(doc, fpus)
	ds = h.mustGet(doc)
	got = ds.Data()
	want = copyMap(want)
	want["*"] = map[string]interface{}{}
	if !testEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_Collections(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}

	doc := integrationColl(t).NewDoc()
	got, err := doc.Collections(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d collections, want 0", len(got))
	}
	var want []*CollectionRef
	for i := 0; i < 3; i++ {
		id := collectionIDs.New()
		cr := doc.Collection(id)
		want = append(want, cr)
		h.mustCreate(cr.NewDoc(), integrationTestMap)
	}
	got, err = doc.Collections(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ServerTimestamp(t *testing.T) {
	type S struct {
		A int
		B time.Time
		C time.Time `firestore:"C.C,serverTimestamp"`
		D map[string]interface{}
		E time.Time `firestore:",omitempty,serverTimestamp"`
	}
	data := S{
		A: 1,
		B: aTime,
		// C is unset, so will get the server timestamp.
		D: map[string]interface{}{"x": ServerTimestamp},
		// E is unset, so will get the server timestamp.
	}
	h := testHelper{t}
	doc := integrationColl(t).NewDoc()
	// Bound times of the RPC, with some slack for clock skew.
	start := time.Now()
	h.mustCreate(doc, data)
	end := time.Now()
	ds := h.mustGet(doc)
	var got S
	if err := ds.DataTo(&got); err != nil {
		t.Fatal(err)
	}
	if !testEqual(got.B, aTime) {
		t.Errorf("B: got %s, want %s", got.B, aTime)
	}
	checkTimeBetween(t, got.C, start, end)
	if g, w := got.D["x"], got.C; !testEqual(g, w) {
		t.Errorf(`D["x"] = %s, want equal to C (%s)`, g, w)
	}
	if g, w := got.E, got.C; !testEqual(g, w) {
		t.Errorf(`E = %s, want equal to C (%s)`, g, w)
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_MergeServerTimestamp(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}

	// Create a doc with an ordinary field "a" and a ServerTimestamp field "b".
	h.mustSet(doc, map[string]interface{}{"a": 1, "b": ServerTimestamp})
	docSnap := h.mustGet(doc)
	data1 := docSnap.Data()
	// Merge with a document with a different value of "a". However,
	// specify only "b" in the list of merge fields.
	h.mustSet(doc, map[string]interface{}{"a": 2, "b": ServerTimestamp}, Merge([]string{"b"}))
	// The result should leave "a" unchanged, while "b" is updated.
	docSnap = h.mustGet(doc)
	data2 := docSnap.Data()
	if got, want := data2["a"], data1["a"]; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t1 := data1["b"].(time.Time)
	t2 := data2["b"].(time.Time)
	if !t1.Before(t2) {
		t.Errorf("got t1=%s, t2=%s; want t1 before t2", t1, t2)
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_MergeNestedServerTimestamp(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}

	// Create a doc with an ordinary field "a" a ServerTimestamp field "b",
	// and a second ServerTimestamp field "c.d".
	h.mustSet(doc, map[string]interface{}{
		"a": 1,
		"b": ServerTimestamp,
		"c": map[string]interface{}{"d": ServerTimestamp},
	})
	data1 := h.mustGet(doc).Data()
	// Merge with a document with a different value of "a". However,
	// specify only "c.d" in the list of merge fields.
	h.mustSet(doc, map[string]interface{}{
		"a": 2,
		"b": ServerTimestamp,
		"c": map[string]interface{}{"d": ServerTimestamp},
	}, Merge([]string{"c", "d"}))
	// The result should leave "a" and "b" unchanged, while "c.d" is updated.
	data2 := h.mustGet(doc).Data()
	if got, want := data2["a"], data1["a"]; got != want {
		t.Errorf("a: got %v, want %v", got, want)
	}
	want := data1["b"].(time.Time)
	got := data2["b"].(time.Time)
	if !got.Equal(want) {
		t.Errorf("b: got %s, want %s", got, want)
	}
	t1 := data1["c"].(map[string]interface{})["d"].(time.Time)
	t2 := data2["c"].(map[string]interface{})["d"].(time.Time)
	if !t1.Before(t2) {
		t.Errorf("got t1=%s, t2=%s; want t1 before t2", t1, t2)
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_WriteBatch(t *testing.T) {
	ctx := context.Background()
	b := integrationClient(t).Batch()
	h := testHelper{t}
	doc1 := iColl.NewDoc()
	doc2 := iColl.NewDoc()
	b.Create(doc1, integrationTestMap)
	b.Set(doc2, integrationTestMap)
	b.Update(doc1, []Update{{Path: "bool", Value: false}})
	b.Update(doc1, []Update{{Path: "str", Value: Delete}})

	wrs, err := b.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(wrs), 4; got != want {
		t.Fatalf("got %d WriteResults, want %d", got, want)
	}
	got1 := h.mustGet(doc1).Data()
	want := copyMap(wantIntegrationTestMap)
	want["bool"] = false
	delete(want, "str")
	if !testEqual(got1, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got1, want)
	}
	got2 := h.mustGet(doc2).Data()
	if !testEqual(got2, wantIntegrationTestMap) {
		t.Errorf("got\n%#v\nwant\n%#v", got2, wantIntegrationTestMap)
	}
	// TODO(jba): test two updates to the same document when it is supported.
	// TODO(jba): test verify when it is supported.

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc1, doc2})
	})
}

func TestIntegration_QueryDocuments_WhereEntity(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	nowTime := time.Now()
	todayTime := nowTime.Unix()
	yesterdayTime := nowTime.AddDate(0, 0, -1).Unix()
	docs := []map[string]interface{}{
		// To support running this test in parallel with the others, use a field name
		// that we don't use anywhere else.
		{"height": 1, "weight": 99, "updatedAt": yesterdayTime},
		{"height": 2, "weight": 98, "updatedAt": yesterdayTime},
		{"height": 3, "weight": 97, "updatedAt": yesterdayTime},
		{"height": 4, "weight": 96, "updatedAt": todayTime},
		{"height": 5, "weight": 95, "updatedAt": todayTime},
		{"height": 6, "weight": 94, "updatedAt": todayTime},
		{"height": 7, "weight": 93, "updatedAt": todayTime},
		{"height": 8, "weight": 93, "updatedAt": todayTime},
	}
	var wants []map[string]interface{}
	var createdDocRefs []*DocumentRef
	for _, doc := range docs {
		newDoc := coll.NewDoc()
		createdDocRefs = append(createdDocRefs, newDoc)
		wants = append(wants, map[string]interface{}{
			"height":    int64(doc["height"].(int)),
			"weight":    int64(doc["weight"].(int)),
			"updatedAt": doc["updatedAt"].(int64),
		})
		h.mustCreate(newDoc, doc)
	}

	q := coll.Select("height", "weight", "updatedAt")
	for i, test := range []struct {
		desc    string
		q       Query
		want    []map[string]interface{}
		orderBy bool // Some query types do not allow ordering.
	}{
		{
			desc: "height == 5",
			q: q.WhereEntity(PropertyFilter{
				Path:     "height",
				Operator: "==",
				Value:    5,
			}),
			want:    wants[4:5],
			orderBy: false,
		},
		{
			desc: "height > 1",
			q: q.WhereEntity(PropertyFilter{
				Path:     "height",
				Operator: ">",
				Value:    1,
			}),
			want:    wants[1:],
			orderBy: true,
		},

		{desc: "((weight > 97 AND updatedAt == yesterdayTime) OR (weight < 94)) AND height == 8",
			q: q.WhereEntity(
				AndFilter{
					Filters: []EntityFilter{
						OrFilter{
							Filters: []EntityFilter{
								AndFilter{
									[]EntityFilter{
										PropertyFilter{Path: "height", Operator: "<", Value: 3},
										PropertyFilter{Path: "updatedAt", Operator: "==", Value: yesterdayTime},
									},
								},
								PropertyFilter{Path: "height", Operator: ">", Value: 6},
							},
						},
						PropertyFilter{Path: "weight", Operator: "==", Value: 93},
					},
				},
			),
			want:    wants[6:],
			orderBy: true,
		},
		{
			desc: "height > 5 OR height < 8",
			q: q.WhereEntity(
				AndFilter{
					Filters: []EntityFilter{
						PropertyFilter{
							Path:     "height",
							Operator: ">",
							Value:    5,
						},
						PropertyFilter{
							Path:     "height",
							Operator: "<",
							Value:    8,
						},
					},
				},
			),
			want:    wants[5:7],
			orderBy: true,
		},
		{
			desc: "height <= 2 OR height > 7",
			q: q.WhereEntity(
				OrFilter{
					Filters: []EntityFilter{
						PropertyFilter{
							Path:     "height",
							Operator: "<=",
							Value:    2,
						},
						PropertyFilter{
							Path:     "height",
							Operator: ">",
							Value:    7,
						},
					},
				},
			),
			want: []map[string]interface{}{
				{"height": int64(1), "weight": int64(99), "updatedAt": int64(yesterdayTime)},
				{"height": int64(2), "weight": int64(98), "updatedAt": int64(yesterdayTime)},
				{"height": int64(8), "weight": int64(93), "updatedAt": int64(todayTime)},
			},
			orderBy: true,
		},
	} {
		if test.orderBy {
			test.q = test.q.OrderBy("height", Asc)
		}
		gotDocs, err := test.q.Documents(ctx).GetAll()
		if err != nil {
			t.Errorf("#%d: %+v: %v", i, test.q, err)
			continue
		}
		if len(gotDocs) != len(test.want) {
			t.Errorf("#%d: (%q) %+v: got %d wants, want %d", i, test.desc, test.q, len(gotDocs), len(test.want))
			continue
		}
		for j, g := range gotDocs {
			if got, want := g.Data(), test.want[j]; !testEqual(got, want) {
				t.Errorf("#%d: %+v, #%d: got\n%+v\nwant\n%+v", i, test.q, j, got, want)
			}
		}
	}
	t.Cleanup(func() {
		deleteDocuments(createdDocRefs)
	})
}

func reverseSlice(s []map[string]interface{}) []map[string]interface{} {
	reversed := make([]map[string]interface{}, len(s))
	for i, j := 0, len(s)-1; i <= j; i, j = i+1, j-1 {
		reversed[i] = s[j]
		reversed[j] = s[i]
	}
	return reversed
}

func TestIntegration_QueryDocuments(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	var wants []map[string]interface{}
	var createdDocRefs []*DocumentRef
	for i := 0; i < 8; i++ {
		doc := coll.NewDoc()
		createdDocRefs = append(createdDocRefs, doc)

		// To support running this test in parallel with the others, use a field name
		// that we don't use anywhere else.
		h.mustCreate(doc, map[string]interface{}{"q": i, "x": 1})
		wants = append(wants, map[string]interface{}{"q": int64(i)})
	}
	q := coll.Select("q")
	for i, test := range []struct {
		desc       string
		q          Query
		want       []map[string]interface{}
		orderBy    bool // Some query types do not allow ordering.
		orderByDir Direction
	}{
		{"Without filters", q, wants, true, 0},
		{"> filter", q.Where("q", ">", 1), wants[2:], true, Asc},
		{"< filter", q.Where("q", "<", 1), wants[:1], true, Asc},
		{"== filter", q.Where("q", "==", 1), wants[1:2], false, 0},
		{"!= filter", q.Where("q", "!=", 0), wants[1:], true, Asc},
		{">= filter", q.Where("q", ">=", 1), wants[1:], true, Asc},
		{"<= filter", q.Where("q", "<=", 1), wants[:2], true, Asc},
		{"in filter", q.Where("q", "in", []int{0}), wants[:1], false, 0},
		{"not-in filter", q.Where("q", "not-in", []int{0, 1}), wants[2:], true, Asc},
		{"WherePath", q.WherePath([]string{"q"}, ">", 1), wants[2:], true, Asc},
		{"Offset with Limit", q.Offset(1).Limit(1), wants[1:2], true, Asc},
		{"StartAt", q.StartAt(1), wants[1:], true, Asc},
		{"StartAfter", q.StartAfter(1), wants[2:], true, Asc},
		{"EndAt", q.EndAt(1), wants[:2], true, Asc},
		{"EndBefore", q.EndBefore(1), wants[:1], true, Asc},
		{"Open range with DESC order", q.StartAfter(6).EndBefore(2), reverseSlice(wants[3:6]), true, Desc},
		{"LimitToLast", q.LimitToLast(2), wants[len(wants)-2:], true, Asc},
		{"StartAfter with LimitToLast", q.StartAfter(2).LimitToLast(2), wants[len(wants)-2:], true, Asc},
		{"StartAt with LimitToLast", q.StartAt(2).LimitToLast(2), wants[len(wants)-2:], true, Asc},
		{"EndBefore with LimitToLast", q.EndBefore(7).LimitToLast(2), wants[5:7], true, Asc},
		{"EndAt with LimitToLast", q.EndAt(7).LimitToLast(2), wants[6:8], true, Asc},
		{"LimitToLast greater than no. of results", q.StartAt(1).EndBefore(2).LimitToLast(3), wants[1:2], true, Asc},
		{"Closed range with LimitToLast ASC order", q.StartAt(2).EndAt(6).LimitToLast(2), wants[5:7], true, Asc},
		{"Left closed right open range with LimitToLast ASC order", q.StartAt(2).EndBefore(6).LimitToLast(2), wants[4:6], true, Asc},
		{"Left open right closed with LimitToLast ASC order", q.StartAfter(2).EndAt(6).LimitToLast(2), wants[5:7], true, Asc},
		{"Open range with LimitToLast ASC order", q.StartAfter(2).EndBefore(6).LimitToLast(2), wants[4:6], true, Asc},
		{"Closed range with LimitToLast DESC order", q.StartAt(6).EndAt(2).LimitToLast(2), reverseSlice(wants[2:4]), true, Desc},
		{"Left closed right open range with LimitToLast DESC order", q.StartAt(6).EndBefore(2).LimitToLast(2), reverseSlice(wants[3:5]), true, Desc},
		{"Left open right closed with LimitToLast DESC order", q.StartAfter(6).EndAt(2).LimitToLast(2), reverseSlice(wants[2:4]), true, Desc},
		{"Open range with LimitToLast DESC order", q.StartAfter(6).EndBefore(2).LimitToLast(2), reverseSlice(wants[3:5]), true, Desc},
	} {
		if test.orderBy {
			test.q = test.q.OrderBy("q", test.orderByDir)
		}
		gotDocs, err := test.q.Documents(ctx).GetAll()
		if err != nil {
			t.Errorf("#%d %v: %+v: %v", i, test.desc, test.q, err)
			continue
		}
		if len(gotDocs) != len(test.want) {
			t.Errorf("#%d %v: %+v: got %d docs, want %d", i, test.desc, test.q, len(gotDocs), len(test.want))
			continue
		}

		fmt.Printf("test.want: %+v\n", test.want)

		docsEqual := true
		docsNotEqualErr := ""
		for j, g := range gotDocs {
			if got, want := g.Data(), test.want[j]; !testEqual(got, want) {
				docsNotEqualErr += fmt.Sprintf("\n\t#%d: got %+v want %+v", j, got, want)
				docsEqual = false
			}
		}
		if !docsEqual {
			t.Errorf("#%d %v: %+v %v", i, test.desc, test.q, docsNotEqualErr)
		}
	}
	_, err := coll.Select("q").Where("x", "==", 1).OrderBy("q", Asc).Documents(ctx).GetAll()
	codeEq(t, "Where and OrderBy on different fields without an index", codes.FailedPrecondition, err)

	// Using the collection itself as the query should return the full documents.
	allDocs, err := coll.Documents(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{} // "q" values we see.
	for _, d := range allDocs {
		data := d.Data()
		q, ok := data["q"]
		if !ok {
			// A document from another test.
			continue
		}
		if seen[q.(int64)] {
			t.Errorf("%v: duplicate doc", data)
		}
		seen[q.(int64)] = true
		if data["x"] != int64(1) {
			t.Errorf("%v: wrong or missing 'x'", data)
		}
		if len(data) != 2 {
			t.Errorf("%v: want two keys", data)
		}
	}
	if got, want := len(seen), len(wants); got != want {
		t.Errorf("got %d docs with 'q', want %d", len(seen), len(wants))
	}

	t.Cleanup(func() {
		deleteDocuments(createdDocRefs)
	})
}

func TestIntegration_QueryDocuments_LimitToLast_Fail(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	q := coll.Select("q").OrderBy("q", Asc).LimitToLast(1)
	got, err := q.Documents(ctx).Next()
	if err == nil {
		t.Errorf("got %v doc, want error", got)
	}
}

// Test unary filters.
func TestIntegration_QueryUnary(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	docRefs := []*DocumentRef{coll.NewDoc(), coll.NewDoc(), coll.NewDoc()}
	h.mustCreate(docRefs[0], map[string]interface{}{"x": 2, "q": "a"})
	h.mustCreate(docRefs[1], map[string]interface{}{"x": 2, "q": nil})
	h.mustCreate(docRefs[2], map[string]interface{}{"x": 2, "q": math.NaN()})
	wantNull := map[string]interface{}{"q": nil}
	wantNaN := map[string]interface{}{"q": math.NaN()}

	base := coll.Select("q").Where("x", "==", 2)
	for _, test := range []struct {
		q    Query
		want map[string]interface{}
	}{
		{base.Where("q", "==", nil), wantNull},
		{base.Where("q", "==", math.NaN()), wantNaN},
	} {
		got, err := test.q.Documents(ctx).GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("got %d responses, want 1", len(got))
			continue
		}
		if g, w := got[0].Data(), test.want; !testEqual(g, w) {
			t.Errorf("%v: got %v, want %v", test.q, g, w)
		}
	}
	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})
}

// Test the special DocumentID field in queries.
func TestIntegration_QueryName(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}

	checkIDs := func(q Query, wantIDs []string) {
		gots, err := q.Documents(ctx).GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(gots) != len(wantIDs) {
			t.Fatalf("got %d, want %d", len(gots), len(wantIDs))
		}
		for i, g := range gots {
			if got, want := g.Ref.ID, wantIDs[i]; got != want {
				t.Errorf("#%d: got %s, want %s", i, got, want)
			}
		}
	}

	coll := integrationColl(t)
	var wantIDs []string
	var docRefs []*DocumentRef
	for i := 0; i < 3; i++ {
		doc := coll.NewDoc()
		docRefs = append(docRefs, doc)
		h.mustCreate(doc, map[string]interface{}{"nm": 1})
		wantIDs = append(wantIDs, doc.ID)
	}
	sort.Strings(wantIDs)
	q := coll.Where("nm", "==", 1).OrderBy(DocumentID, Asc)
	checkIDs(q, wantIDs)

	// Empty Select.
	q = coll.Select().Where("nm", "==", 1).OrderBy(DocumentID, Asc)
	checkIDs(q, wantIDs)

	// Test cursors with __name__.
	checkIDs(q.StartAt(wantIDs[1]), wantIDs[1:])
	checkIDs(q.EndAt(wantIDs[1]), wantIDs[:2])

	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})
}

func TestIntegration_QueryNested(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}
	coll1 := integrationColl(t)
	doc1 := coll1.NewDoc()
	coll2 := doc1.Collection(collectionIDs.New())
	doc2 := coll2.NewDoc()
	wantData := map[string]interface{}{"x": int64(1)}
	h.mustCreate(doc2, wantData)
	q := coll2.Select("x")
	got, err := q.Documents(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d docs, want 1", len(got))
	}
	if gotData := got[0].Data(); !testEqual(gotData, wantData) {
		t.Errorf("got\n%+v\nwant\n%+v", gotData, wantData)
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc1, doc2})
	})
}

func TestIntegration_RunTransaction(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}

	type Player struct {
		Name  string
		Score int
		Star  bool `firestore:"*"`
	}

	pat := Player{Name: "Pat", Score: 3, Star: false}
	client := integrationClient(t)
	patDoc := iColl.Doc("pat")
	var anError error
	incPat := func(_ context.Context, tx *Transaction) error {
		doc, err := tx.Get(patDoc)
		if err != nil {
			return err
		}
		score, err := doc.DataAt("Score")
		if err != nil {
			return err
		}
		// Since the Star field is called "*", we must use DataAtPath to get it.
		star, err := doc.DataAtPath([]string{"*"})
		if err != nil {
			return err
		}
		err = tx.Update(patDoc, []Update{{Path: "Score", Value: int(score.(int64) + 7)}})
		if err != nil {
			return err
		}
		// Since the Star field is called "*", we must use Update to change it.
		err = tx.Update(patDoc,
			[]Update{{FieldPath: []string{"*"}, Value: !star.(bool)}})
		if err != nil {
			return err
		}
		return anError
	}

	h.mustCreate(patDoc, pat)
	err := client.RunTransaction(ctx, incPat)
	if err != nil {
		t.Fatal(err)
	}
	ds := h.mustGet(patDoc)
	var got Player
	if err := ds.DataTo(&got); err != nil {
		t.Fatal(err)
	}
	want := Player{Name: "Pat", Score: 10, Star: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// Function returns error, so transaction is rolled back and no writes happen.
	anError = errors.New("bad")
	err = client.RunTransaction(ctx, incPat)
	if err != anError {
		t.Fatalf("got %v, want %v", err, anError)
	}
	if err := ds.DataTo(&got); err != nil {
		t.Fatal(err)
	}
	// want is same as before.
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{patDoc})
	})
}

func TestIntegration_TransactionGetAll(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}
	type Player struct {
		Name  string
		Score int
	}
	lee := Player{Name: "Lee", Score: 3}
	sam := Player{Name: "Sam", Score: 1}
	client := integrationClient(t)
	leeDoc := iColl.Doc("lee")
	samDoc := iColl.Doc("sam")
	h.mustCreate(leeDoc, lee)
	h.mustCreate(samDoc, sam)

	err := client.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		docs, err := tx.GetAll([]*DocumentRef{samDoc, leeDoc})
		if err != nil {
			return err
		}
		for i, want := range []Player{sam, lee} {
			var got Player
			if err := docs[i].DataTo(&got); err != nil {
				return err
			}
			if !testutil.Equal(got, want) {
				return fmt.Errorf("got %+v, want %+v", got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{leeDoc, samDoc})
	})
}

func TestIntegration_WatchDocument(t *testing.T) {
	coll := integrationColl(t)
	ctx := context.Background()
	h := testHelper{t}
	doc := coll.NewDoc()
	it := doc.Snapshots(ctx)
	defer it.Stop()

	next := func() *DocumentSnapshot {
		snap, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}

	snap := next()
	if snap.Exists() {
		t.Fatal("snapshot exists; it should not")
	}
	want := map[string]interface{}{"a": int64(1), "b": "two"}
	h.mustCreate(doc, want)
	snap = next()
	if got := snap.Data(); !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	h.mustUpdate(doc, []Update{{Path: "a", Value: int64(2)}})
	want["a"] = int64(2)
	snap = next()
	if got := snap.Data(); !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	h.mustDelete(doc)
	snap = next()
	if snap.Exists() {
		t.Fatal("snapshot exists; it should not")
	}

	h.mustCreate(doc, want)
	snap = next()
	if got := snap.Data(); !testutil.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ArrayUnion_Create(t *testing.T) {
	path := "somePath"
	data := map[string]interface{}{
		path: ArrayUnion("a", "b"),
	}

	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	h.mustCreate(doc, data)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "b"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ArrayUnion_Update(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	path := "somePath"

	h.mustCreate(doc, map[string]interface{}{
		path: []string{"a", "b"},
	})
	fpus := []Update{
		{
			Path:  path,
			Value: ArrayUnion("this should be added"),
		},
	}
	h.mustUpdate(doc, fpus)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "b", "this should be added"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}
	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ArrayUnion_Set(t *testing.T) {
	coll := integrationColl(t)
	h := testHelper{t}
	path := "somePath"

	doc := coll.NewDoc()
	newData := map[string]interface{}{
		path: ArrayUnion("a", "b"),
	}
	h.mustSet(doc, newData)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "b"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ArrayRemove_Create(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	path := "somePath"

	h.mustCreate(doc, map[string]interface{}{
		path: ArrayRemove("a", "b"),
	})

	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	// A create with arrayRemove results in an empty array.
	want := []string(nil)
	if !testEqual(gotMap[path], want) {
		t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ArrayRemove_Update(t *testing.T) {
	doc := integrationColl(t).NewDoc()
	h := testHelper{t}
	path := "somePath"

	h.mustCreate(doc, map[string]interface{}{
		path: []string{"a", "this should be removed", "c"},
	})
	fpus := []Update{
		{
			Path:  path,
			Value: ArrayRemove("this should be removed"),
		},
	}
	h.mustUpdate(doc, fpus)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string{"a", "c"}
	for i, v := range gotMap[path] {
		if v != want[i] {
			t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
		}
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_ArrayRemove_Set(t *testing.T) {
	coll := integrationColl(t)
	h := testHelper{t}
	path := "somePath"

	doc := coll.NewDoc()
	newData := map[string]interface{}{
		path: ArrayRemove("a", "b"),
	}
	h.mustSet(doc, newData)
	ds := h.mustGet(doc)
	var gotMap map[string][]string
	if err := ds.DataTo(&gotMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotMap[path]; !ok {
		t.Fatalf("expected a %v key in data, got %v", path, gotMap)
	}

	want := []string(nil)
	if !testEqual(gotMap[path], want) {
		t.Fatalf("got\n%#v\nwant\n%#v", gotMap[path], want)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func makeFieldTransform(transform string, value interface{}) interface{} {
	switch transform {
	case "inc":
		return FieldTransformIncrement(value)
	case "max":
		return FieldTransformMaximum(value)
	case "min":
		return FieldTransformMinimum(value)
	}
	panic(fmt.Sprintf("Invalid transform %v", transform))
}

func TestIntegration_FieldTransforms_Create(t *testing.T) {
	for _, transform := range []string{"inc", "max", "min"} {
		t.Run(transform, func(t *testing.T) {
			doc := integrationColl(t).NewDoc()
			h := testHelper{t}
			path := "somePath"
			want := 7

			h.mustCreate(doc, map[string]interface{}{
				path: makeFieldTransform(transform, want),
			})

			ds := h.mustGet(doc)
			var gotMap map[string]int
			if err := ds.DataTo(&gotMap); err != nil {
				t.Fatal(err)
			}
			if _, ok := gotMap[path]; !ok {
				t.Fatalf("expected a %v key in data, got %v", path, gotMap)
			}

			if gotMap[path] != want {
				t.Fatalf("want %d, got %d", want, gotMap[path])
			}

			t.Cleanup(func() {
				deleteDocuments([]*DocumentRef{doc})
			})
		})
	}
}

// Also checks that all appropriate types are supported.
func TestIntegration_FieldTransforms_Update(t *testing.T) {
	type MyInt = int // Test a custom type.
	for _, tc := range []struct {
		// All three should be same type.
		start interface{}
		val   interface{}
		inc   interface{}
		max   interface{}
		min   interface{}

		wantErr bool
	}{
		{start: int(7), val: int(4), inc: int(11), max: int(7), min: int(4)},
		{start: int8(2), val: int8(4), inc: int8(6), max: int8(4), min: int8(2)},
		{start: int16(7), val: int16(4), inc: int16(11), max: int16(7), min: int16(4)},
		{start: int32(7), val: int32(4), inc: int32(11), max: int32(7), min: int32(4)},
		{start: int64(7), val: int64(4), inc: int64(11), max: int64(7), min: int64(4)},
		{start: uint8(7), val: uint8(4), inc: uint8(11), max: uint8(7), min: uint8(4)},
		{start: uint16(7), val: uint16(4), inc: uint16(11), max: int16(7), min: int16(4)},
		{start: uint32(7), val: uint32(4), inc: uint32(11), max: int32(7), min: int32(4)},
		{start: float32(7.7), val: float32(4.1), inc: float32(11.8), max: float32(7.7), min: float32(4.1)},
		{start: float64(2.2), val: float64(4.1), inc: float64(6.3), max: float64(4.1), min: float64(2.2)},
		{start: MyInt(7), val: MyInt(4), inc: MyInt(11), max: MyInt(7), min: MyInt(4)},
		{start: 7, val: "strings are not allowed", wantErr: true},
		{start: 7, val: uint(3), wantErr: true},
		{start: 7, val: uint64(3), wantErr: true},
	} {
		for _, transform := range []string{"inc", "max", "min"} {
			t.Run(transform, func(t *testing.T) {
				typeStr := reflect.TypeOf(tc.val).String()
				t.Run(typeStr, func(t *testing.T) {
					doc := integrationColl(t).NewDoc()
					h := testHelper{t}
					path := "somePath"

					h.mustCreate(doc, map[string]interface{}{
						path: tc.start,
					})
					fpus := []Update{
						{
							Path:  path,
							Value: makeFieldTransform(transform, tc.val),
						},
					}
					_, err := doc.Update(context.Background(), fpus)
					if err != nil {
						if tc.wantErr {
							return
						}
						h.t.Fatalf("%s: updating: %v", loc(), err)
					}
					ds := h.mustGet(doc)
					var gotMap map[string]interface{}
					if err := ds.DataTo(&gotMap); err != nil {
						t.Fatal(err)
					}

					var want interface{}
					switch transform {
					case "inc":
						want = tc.inc
					case "max":
						want = tc.max
					case "min":
						want = tc.min
					default:
						t.Fatalf("unsupported transform type %s", transform)
					}

					switch want.(type) {
					case int, int8, int16, int32, int64:
						if _, ok := gotMap[path]; !ok {
							t.Fatalf("expected a %v key in data, got %v", path, gotMap)
						}
						if got, want := reflect.ValueOf(gotMap[path]).Int(), reflect.ValueOf(want).Int(); got != want {
							t.Fatalf("want %v, got %v", want, got)
						}
					case uint8, uint16, uint32:
						if _, ok := gotMap[path]; !ok {
							t.Fatalf("expected a %v key in data, got %v", path, gotMap)
						}
						if got, want := uint64(reflect.ValueOf(gotMap[path]).Int()), reflect.ValueOf(want).Uint(); got != want {
							t.Fatalf("want %v, got %v", want, got)
						}
					case float32, float64:
						if _, ok := gotMap[path]; !ok {
							t.Fatalf("expected a %v key in data, got %v", path, gotMap)
						}
						const precision = 1e-6 // Floats are never precisely comparable.
						if got, want := reflect.ValueOf(gotMap[path]).Float(), reflect.ValueOf(want).Float(); math.Abs(got-want) > precision {
							t.Fatalf("want %v, got %v", want, got)
						}
					default:
						// Either some unsupported type was added without specifying
						// wantErr, or a supported type needs to be added to this
						// switch statement.
						t.Fatalf("unsupported type %T", want)
					}

					t.Cleanup(func() {
						deleteDocuments([]*DocumentRef{doc})
					})
				})
			})
		}
	}
}

func TestIntegration_FieldTransforms_Set(t *testing.T) {
	for _, transform := range []string{"inc", "max", "min"} {
		t.Run(transform, func(t *testing.T) {
			coll := integrationColl(t)
			h := testHelper{t}
			path := "somePath"
			want := 9

			doc := coll.NewDoc()
			newData := map[string]interface{}{
				path: makeFieldTransform(transform, want),
			}
			h.mustSet(doc, newData)
			ds := h.mustGet(doc)
			var gotMap map[string]int
			if err := ds.DataTo(&gotMap); err != nil {
				t.Fatal(err)
			}
			if _, ok := gotMap[path]; !ok {
				t.Fatalf("expected a %v key in data, got %v", path, gotMap)
			}

			if gotMap[path] != want {
				t.Fatalf("want %d, got %d", want, gotMap[path])
			}

			t.Cleanup(func() {
				deleteDocuments([]*DocumentRef{doc})
			})
		})
	}
}

type imap map[string]interface{}

func TestIntegration_Serialize_Deserialize_WatchQuery(t *testing.T) {
	h := testHelper{t}
	collID := collectionIDs.New()
	ctx := context.Background()
	client := integrationClient(t)

	partitionedQueries, err := client.CollectionGroup(collID).GetPartitionedQueries(ctx, 10)
	h.failIfNotNil(err)

	qProtoBytes, err := partitionedQueries[0].Serialize()
	h.failIfNotNil(err)

	q, err := client.CollectionGroup(collID).Deserialize(qProtoBytes)
	h.failIfNotNil(err)

	qSnapIt := q.Snapshots(ctx)
	defer qSnapIt.Stop()

	// Check if at least one snapshot exists
	_, err = qSnapIt.Next()
	if err == iterator.Done {
		t.Fatalf("Expected snapshot, found none")
	}

	// Add new document to query results
	createdDocRefs := h.mustCreateMulti(collID, []testDocument{
		{data: map[string]interface{}{"some-key": "should-be-found"}},
	})
	wds := h.mustGet(createdDocRefs[0])

	// Check if new snapshot is available
	qSnap, err := qSnapIt.Next()
	if err == iterator.Done {
		t.Fatalf("Expected snapshot, found none")
	}

	// Check the changes in snapshot
	if len(qSnap.Changes) != 1 {
		t.Fatalf("Expected one change, found none")
	}

	wantChange := DocumentChange{Kind: DocumentAdded, Doc: wds, OldIndex: -1, NewIndex: 0}
	gotChange := qSnap.Changes[0]
	copts := append([]cmp.Option{cmpopts.IgnoreFields(DocumentSnapshot{}, "ReadTime")}, cmpOpts...)
	if diff := testutil.Diff(gotChange, wantChange, copts...); diff != "" {
		t.Errorf("got: %v, want: %v, diff: %v", gotChange, wantChange, diff)
	}
}

func TestIntegration_WatchQuery(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}

	q := coll.Where("e", ">", 1).OrderBy("e", Asc)
	it := q.Snapshots(ctx)
	defer it.Stop()

	next := func() ([]*DocumentSnapshot, []DocumentChange) {
		qsnap, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		if qsnap.ReadTime.IsZero() {
			t.Fatal("zero time")
		}
		ds, err := qsnap.Documents.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if qsnap.Size != len(ds) {
			t.Fatalf("Size=%d but we have %d docs", qsnap.Size, len(ds))
		}
		return ds, qsnap.Changes
	}

	copts := append([]cmp.Option{cmpopts.IgnoreFields(DocumentSnapshot{}, "ReadTime")}, cmpOpts...)
	check := func(msg string, wantd []*DocumentSnapshot, wantc []DocumentChange) {
		gotd, gotc := next()
		if diff := testutil.Diff(gotd, wantd, copts...); diff != "" {
			t.Errorf("%s: %s", msg, diff)
		}
		if diff := testutil.Diff(gotc, wantc, copts...); diff != "" {
			t.Errorf("%s: %s", msg, diff)
		}
	}

	check("initial", nil, nil)
	doc1 := coll.NewDoc()
	h.mustCreate(doc1, imap{"e": int64(2), "b": "two"})
	wds := h.mustGet(doc1)
	check("one",
		[]*DocumentSnapshot{wds},
		[]DocumentChange{{Kind: DocumentAdded, Doc: wds, OldIndex: -1, NewIndex: 0}})

	// Add a doc that does not match. We won't see a snapshot  for this.
	doc2 := coll.NewDoc()
	h.mustCreate(doc2, imap{"e": int64(1)})

	// Update the first doc. We should see the change. We won't see doc2.
	h.mustUpdate(doc1, []Update{{Path: "e", Value: int64(3)}})
	wds = h.mustGet(doc1)
	check("update",
		[]*DocumentSnapshot{wds},
		[]DocumentChange{{Kind: DocumentModified, Doc: wds, OldIndex: 0, NewIndex: 0}})

	// Now update doc so that it is not in the query. We should see a snapshot with no docs.
	h.mustUpdate(doc1, []Update{{Path: "e", Value: int64(0)}})
	check("update2", nil, []DocumentChange{{Kind: DocumentRemoved, Doc: wds, OldIndex: 0, NewIndex: -1}})

	// Add two docs out of order. We should see them in order.
	doc3 := coll.NewDoc()
	doc4 := coll.NewDoc()
	want3 := imap{"e": int64(5)}
	want4 := imap{"e": int64(4)}
	h.mustCreate(doc3, want3)
	h.mustCreate(doc4, want4)
	wds4 := h.mustGet(doc4)
	wds3 := h.mustGet(doc3)
	check("two#1",
		[]*DocumentSnapshot{wds3},
		[]DocumentChange{{Kind: DocumentAdded, Doc: wds3, OldIndex: -1, NewIndex: 0}})
	check("two#2",
		[]*DocumentSnapshot{wds4, wds3},
		[]DocumentChange{{Kind: DocumentAdded, Doc: wds4, OldIndex: -1, NewIndex: 0}})
	// Delete a doc.
	h.mustDelete(doc4)
	check("after del", []*DocumentSnapshot{wds3}, []DocumentChange{{Kind: DocumentRemoved, Doc: wds4, OldIndex: 0, NewIndex: -1}})

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc1, doc2, doc3})
	})
}

func TestIntegration_WatchQueryCancel(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)

	q := coll.Where("e", ">", 1).OrderBy("e", Asc)
	ctx, cancel := context.WithCancel(ctx)
	it := q.Snapshots(ctx)
	defer it.Stop()

	// First call opens the stream.
	_, err := it.Next()
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, err = it.Next()
	codeEq(t, "after cancel", codes.Canceled, err)
}

func TestIntegration_MissingDocs(t *testing.T) {
	ctx := context.Background()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	dr1 := coll.NewDoc()
	dr2 := coll.NewDoc()
	dr3 := dr2.Collection("sub").NewDoc()
	h.mustCreate(dr1, integrationTestMap)
	defer h.mustDelete(dr1)
	h.mustCreate(dr3, integrationTestMap)
	defer h.mustDelete(dr3)

	// dr1 is a document in coll. dr2 was never created, but there are documents in
	// its sub-collections. It is "missing".
	// The Collection.DocumentRefs method includes missing document refs.
	want := []string{dr1.Path, dr2.Path}
	drs, err := coll.DocumentRefs(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, dr := range drs {
		got = append(got, dr.Path)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !testutil.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{dr2})
	})
}

func TestIntegration_CollectionGroupQueries(t *testing.T) {
	shouldBeFoundID := collectionIDs.New()
	shouldNotBeFoundID := collectionIDs.New()

	ctx := context.Background()
	h := testHelper{t}
	client := integrationClient(t)
	cr1 := client.Collection(shouldBeFoundID)
	dr1 := cr1.Doc("should-be-found-1")
	h.mustCreate(dr1, map[string]string{"some-key": "should-be-found"})
	defer h.mustDelete(dr1)

	dr1.Collection(shouldBeFoundID)
	dr2 := cr1.Doc("should-be-found-2")
	h.mustCreate(dr2, map[string]string{"some-key": "should-be-found"})
	defer h.mustDelete(dr2)

	cr3 := client.Collection(shouldNotBeFoundID)
	dr3 := cr3.Doc("should-not-be-found")
	h.mustCreate(dr3, map[string]string{"some-key": "should-NOT-be-found"})
	defer h.mustDelete(dr3)

	cg := client.CollectionGroup(shouldBeFoundID)
	snaps, err := cg.Documents(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots but got %d", len(snaps))
	}
	if snaps[0].Ref.ID != "should-be-found-1" {
		t.Fatalf("expected ID 'should-be-found-1', got %s", snaps[0].Ref.ID)
	}
	if snaps[1].Ref.ID != "should-be-found-2" {
		t.Fatalf("expected ID 'should-be-found-2', got %s", snaps[1].Ref.ID)
	}
}

func codeEq(t *testing.T, msg string, code codes.Code, err error) {
	if status.Code(err) != code {
		t.Fatalf("%s:\ngot <%v>\nwant code %s", msg, err, code)
	}
}

func loc() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "???"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	c := map[string]interface{}{}
	for k, v := range m {
		c[k] = v
	}
	return c
}

func checkTimeBetween(t *testing.T, got, low, high time.Time) {
	// Allow slack for clock skew.
	const slack = 4 * time.Second
	low = low.Add(-slack)
	high = high.Add(slack)
	if got.Before(low) || got.After(high) {
		t.Fatalf("got %s, not in [%s, %s]", got, low, high)
	}
}

type testHelper struct {
	t *testing.T
}

func (h testHelper) failIfNotNil(err error) {
	if err != nil {
		h.t.Fatal(err)
	}
}

type testDocument struct {
	id   string
	data map[string]interface{}
}

func (h testHelper) mustCreateMulti(collectionPath string, docsData []testDocument) []*DocumentRef {
	client := integrationClient(h.t)
	collRef := client.Collection(collectionPath)
	docsCreated := []*DocumentRef{}
	for _, data := range docsData {
		var docRef *DocumentRef
		if len(data.id) == 0 {
			docRef = collRef.NewDoc()
		} else {
			docRef = collRef.Doc(data.id)
		}
		h.mustCreate(docRef, data.data)
		docsCreated = append(docsCreated, docRef)
	}

	h.t.Cleanup(func() {
		deleteDocuments(docsCreated)
	})

	return docsCreated
}

func (h testHelper) mustCreate(doc *DocumentRef, data interface{}) *WriteResult {
	wr, err := doc.Create(context.Background(), data)
	if err != nil {
		h.t.Fatalf("%s: creating: %v", loc(), err)
	}
	return wr
}

func (h testHelper) mustUpdate(doc *DocumentRef, updates []Update) *WriteResult {
	wr, err := doc.Update(context.Background(), updates)
	if err != nil {
		h.t.Fatalf("%s: updating: %v", loc(), err)
	}
	return wr
}

func (h testHelper) mustGet(doc *DocumentRef) *DocumentSnapshot {
	d, err := doc.Get(context.Background())
	if err != nil {
		h.t.Fatalf("%s: getting: %v", loc(), err)
	}
	return d
}

func (h testHelper) mustDelete(doc *DocumentRef) *WriteResult {
	wr, err := doc.Delete(context.Background())
	if err != nil {
		h.t.Fatalf("%s: updating: %v", loc(), err)
	}
	return wr
}

func (h testHelper) mustSet(doc *DocumentRef, data interface{}, opts ...SetOption) *WriteResult {
	wr, err := doc.Set(context.Background(), data, opts...)
	if err != nil {
		h.t.Fatalf("%s: updating: %v", loc(), err)
	}
	return wr
}

func TestDetectProjectID(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()

	creds := testutil.Credentials(ctx)
	if creds == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	// Use creds with project ID.
	if _, err := NewClient(ctx, DetectProjectID, option.WithCredentials(creds)); err != nil {
		t.Errorf("NewClient: %v", err)
	}

	ts := testutil.ErroringTokenSource{}
	// Try to use creds without project ID.
	_, err := NewClient(ctx, DetectProjectID, option.WithTokenSource(ts))
	if err == nil || err.Error() != "firestore: see the docs on DetectProjectID" {
		t.Errorf("expected an error while using TokenSource that does not have a project ID")
	}
}

func TestIntegration_ColGroupRefPartitions(t *testing.T) {
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	ctx := context.Background()

	// Create a doc in the test collection so a collectionID is live for testing
	doc := coll.NewDoc()
	h.mustCreate(doc, integrationTestMap)
	defer doc.Delete(ctx)

	// Verify partitions are within an expected range. Paritioning isn't exact
	// so a fuzzy count needs to be used.
	for idx, tc := range []struct {
		collectionID              string
		minExpectedPartitionCount int
		maxExpectedPartitionCount int
	}{
		// Verify no failures if a collection doesn't exist
		{collectionID: "does-not-exist", minExpectedPartitionCount: 1, maxExpectedPartitionCount: 1},
		// Verify a collectionID with a small number of results returns a partition
		{collectionID: coll.collectionID, minExpectedPartitionCount: 1, maxExpectedPartitionCount: 2},
	} {
		colGroup := iClient.CollectionGroup(tc.collectionID)
		partitions, err := colGroup.GetPartitionedQueries(ctx, 10)
		if err != nil {
			t.Fatalf("getPartitions: received unexpected error: %v", err)
		}
		got, minWant, maxWant := len(partitions), tc.minExpectedPartitionCount, tc.maxExpectedPartitionCount
		if got < minWant || got > maxWant {
			t.Errorf(
				"Unexpected Partition Count:index:%d, got %d, want min:%d max:%d",
				idx, got, minWant, maxWant,
			)
			for _, v := range partitions {
				t.Errorf(
					"Partition: startDoc:%v, endDoc:%v, startVals:%v, endVals:%v",
					v.startDoc, v.endDoc, v.startVals, v.endVals,
				)
			}
		}
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})

}

func TestIntegration_ColGroupRefPartitionsLarge(t *testing.T) {
	// Create collection with enough documents to have multiple partitions.
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	collectionID := coll.collectionID

	ctx := context.Background()

	documentCount := 2*128 + 127 // Minimum partition size is 128.

	// Create documents in a collection sufficient to trigger multiple partitions.
	batch := iClient.Batch()
	deleteBatch := iClient.Batch()
	for i := 0; i < documentCount; i++ {
		doc := coll.Doc(fmt.Sprintf("doc%d", i))
		batch.Create(doc, integrationTestMap)
		deleteBatch.Delete(doc)
	}
	batch.Commit(ctx)
	defer deleteBatch.Commit(ctx)

	// Verify that we retrieve 383 documents for the colGroup (128*2 + 127)
	colGroup := iClient.CollectionGroup(collectionID)
	docs, err := colGroup.Documents(ctx).GetAll()
	if err != nil {
		t.Fatalf("GetAll(): received unexpected error: %v", err)
	}
	if got, want := len(docs), documentCount; got != want {
		t.Errorf("Unexpected number of documents in collection group: got %d, want %d", got, want)
	}

	// Get partitions, allow up to 10 to come back, expect less will be returned.
	partitions, err := colGroup.GetPartitionedQueries(ctx, 10)
	if err != nil {
		t.Fatalf("GetPartitionedQueries: received unexpected error: %v", err)
	}
	if len(partitions) < 2 {
		t.Errorf("Unexpected Partition Count. Expected 2 or more: got %d, want 2+", len(partitions))
	}

	// Verify that we retrieve 383 documents across all partitions. (128*2 + 127)
	totalCount := 0
	for _, query := range partitions {
		allDocs, err := query.Documents(ctx).GetAll()
		if err != nil {
			t.Fatalf("GetAll(): received unexpected error: %v", err)
		}
		totalCount += len(allDocs)

		// Verify that serialization round-trips. Check that the same results are
		// returned even if we use the proto converted query
		queryBytes, err := query.Serialize()
		if err != nil {
			t.Fatalf("Serialize error: %v", err)
		}
		q, err := iClient.CollectionGroup("DNE").Deserialize(queryBytes)
		if err != nil {
			t.Fatalf("Deserialize error: %v", err)
		}

		protoReturnedDocs, err := q.Documents(ctx).GetAll()
		if err != nil {
			t.Fatalf("GetAll error: %v", err)
		}
		if len(allDocs) != len(protoReturnedDocs) {
			t.Fatalf("Expected document count to be the same on both query runs: %v", err)
		}

		t.Cleanup(func() {
			deleteDocumentSnapshots(allDocs)
		})
	}

	if got, want := totalCount, documentCount; got != want {
		t.Errorf("Unexpected number of documents across partitions: got %d, want %d", got, want)
	}
}

func TestIntegration_NewClientWithDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	for _, tc := range []struct {
		desc    string
		dbName  string
		wantErr bool
		opt     []option.ClientOption
	}{
		{
			desc:    "Success",
			dbName:  testParams["databaseID"].(string),
			wantErr: false,
		},
		{
			desc:    "Error from NewClient bubbled to NewClientWithDatabase",
			dbName:  testParams["databaseID"].(string),
			wantErr: true,
			opt:     []option.ClientOption{option.WithCredentialsFile("non existent filepath")},
		},
	} {
		ctx := context.Background()
		c, err := NewClientWithDatabase(ctx, iClient.projectID, tc.dbName, tc.opt...)
		if err != nil && !tc.wantErr {
			t.Errorf("NewClientWithDatabase: %s got %v want nil", tc.desc, err)
		} else if err == nil && tc.wantErr {
			t.Errorf("NewClientWithDatabase: %s got %v wanted error", tc.desc, err)
		} else if err == nil && c.databaseID != tc.dbName {
			t.Errorf("NewClientWithDatabase: %s got %v want %v", tc.desc, c.databaseID, tc.dbName)
		}
	}
}

// TestIntegration_BulkWriter_Set tests setting values and serverTimeStamp in single write.
func TestIntegration_BulkWriter_Set(t *testing.T) {
	doc := iColl.NewDoc()
	c := integrationClient(t)
	ctx := context.Background()
	bw := c.BulkWriter(ctx)

	f := copyMap(integrationTestMap)
	f["serverTimeStamp"] = ServerTimestamp
	_, err := bw.Set(doc, f)
	if err != nil {
		t.Errorf("bulkwriter: error performing a set write: %v\n", err)
	}

	t.Cleanup(func() {
		deleteDocuments([]*DocumentRef{doc})
	})
}

func TestIntegration_BulkWriter_Create(t *testing.T) {
	c := integrationClient(t)
	ctx := context.Background()

	type BWDoc struct {
		A int
	}

	docRef := iColl.Doc(fmt.Sprintf("bw_create_1_%d", time.Now().Unix()))
	_, err := docRef.Create(ctx, BWDoc{A: 6})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	doc := BWDoc{A: 5}
	testcases := []struct {
		desc           string
		ref            *DocumentRef
		wantStatusCode codes.Code
	}{
		{
			desc:           "Successful",
			ref:            iColl.Doc(fmt.Sprintf("bw_create_2_%d", time.Now().Unix())),
			wantStatusCode: codes.OK,
		},
		{
			desc:           "Already exists error",
			ref:            docRef,
			wantStatusCode: codes.AlreadyExists,
		},
	}
	for _, testcase := range testcases {
		bw := c.BulkWriter(ctx)

		bwJob, err := bw.Create(testcase.ref, doc)
		if err != nil {
			t.Errorf("%v Create %v", testcase.desc, err)
			continue
		}
		bw.Flush()

		_, gotErr := bwJob.Results()
		if status.Code(gotErr) != testcase.wantStatusCode {
			t.Errorf("%q: Mismatch in error got: %v, want: %q", testcase.desc, status.Code(gotErr), testcase.wantStatusCode)
		}
	}
}

func TestIntegration_BulkWriter(t *testing.T) {
	doc := iColl.NewDoc()
	docRefs := []*DocumentRef{doc}
	c := integrationClient(t)
	ctx := context.Background()
	bw := c.BulkWriter(ctx)

	f := integrationTestMap
	j, err := bw.Create(doc, f)

	if err != nil {
		t.Errorf("bulkwriter: error creating write to database: %v\n", err)
	}

	bw.Flush()              // This blocks
	res, err := j.Results() // so does this

	if err != nil {
		t.Errorf("bulkwriter: error getting write results: %v\n", err)
	}

	if res == nil {
		t.Error("bulkwriter: write attempt returned nil results")
	}

	numNewWrites := 21 // 20 is the threshold at which the bundler should start sending requests
	var jobs []*BulkWriterJob

	// Test a slew of writes sent at the BulkWriter
	for i := 0; i < numNewWrites; i++ {
		d := iColl.NewDoc()
		docRefs = append(docRefs, d)
		jb, err := bw.Create(d, f)

		if err != nil {
			t.Errorf("bulkwriter: error creating write to database: %v\n", err)
		}

		jobs = append(jobs, jb)
	}

	bw.End() // This calls Flush() in the background.

	for _, j := range jobs {
		res, err = j.Results()
		if err != nil {
			t.Errorf("bulkwriter: error getting write results: %v\n", err)
		}

		if res == nil {
			t.Error("bulkwriter: write attempt returned nil results")
		}
	}
	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})
}

func TestIntegration_AggregationQueries(t *testing.T) {
	ctx := context.Background()
	coll := integrationColl(t)
	client := integrationClient(t)
	h := testHelper{t}
	docs := []map[string]interface{}{
		{"width": 1.5, "depth": 99, "model": "A"},
		{"width": 2.6, "depth": 98, "model": "A"},
		{"width": 3.7, "depth": 97, "model": "B"},
		{"width": 4.8, "depth": 96, "model": "B"},
		{"width": 5.9, "depth": 95, "model": "C"},
		{"width": 6.0, "depth": 94, "model": "B"},
		{"width": 7.1, "depth": 93, "model": "C"},
		{"width": 8.2, "depth": 93, "model": "A"},
	}
	for _, doc := range docs {
		newDoc := coll.NewDoc()
		h.mustCreate(newDoc, doc)
	}

	query := coll.Where("width", ">=", 1)

	limitQuery := coll.Where("width", ">=", 1).Limit(4)
	limitToLastQuery := coll.Where("width", ">=", 2.6).OrderBy("width", Asc).LimitToLast(4)

	startAtQuery := coll.Where("width", ">=", 2.6).OrderBy("width", Asc).StartAt(3.7)
	startAfterQuery := coll.Where("width", ">=", 2.6).OrderBy("width", Asc).StartAfter(3.7)

	endAtQuery := coll.Where("width", ">=", 2.6).OrderBy("width", Asc).EndAt(7.1)
	endBeforeQuery := coll.Where("width", ">=", 2.6).OrderBy("width", Asc).EndBefore(7.1)

	emptyResultsQuery := coll.Where("width", "<", 1)
	emptyResultsQueryPtr := &emptyResultsQuery

	testcases := []struct {
		desc             string
		aggregationQuery *AggregationQuery
		wantErr          bool
		runInTransaction bool
		result           AggregationResult
	}{
		{
			desc:             "Multiple aggregations",
			aggregationQuery: query.NewAggregationQuery().WithCount("count1").WithAvg("width", "width_avg1").WithAvg("depth", "depth_avg1").WithSum("width", "width_sum1").WithSum("depth", "depth_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(8)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(39.8)}},
				"depth_sum1": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(765)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(4.975)}},
				"depth_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(95.625)}},
			},
		},
		{
			desc:             "Aggregations in transaction",
			aggregationQuery: query.NewAggregationQuery().WithCount("count1").WithAvg("width", "width_avg1").WithAvg("depth", "depth_avg1").WithSum("width", "width_sum1").WithSum("depth", "depth_sum1"),
			wantErr:          false,
			runInTransaction: true,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(8)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(39.8)}},
				"depth_sum1": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(765)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(4.975)}},
				"depth_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(95.625)}},
			},
		},
		{
			desc:             "WithSum aggregation without alias",
			aggregationQuery: query.NewAggregationQuery().WithSum("width", ""),
			wantErr:          false,
			result: map[string]interface{}{
				"field_1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(39.8)}},
			},
		},
		{
			desc:             "WithSumPath aggregation without alias",
			aggregationQuery: query.NewAggregationQuery().WithSumPath([]string{"width"}, ""),
			wantErr:          false,
			result: map[string]interface{}{
				"field_1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(39.8)}},
			},
		},
		{
			desc:             "WithAvg aggregation without alias",
			aggregationQuery: query.NewAggregationQuery().WithAvg("width", ""),
			wantErr:          false,
			result: map[string]interface{}{
				"field_1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(4.975)}},
			},
		},
		{
			desc:             "WithAvgPath aggregation without alias",
			aggregationQuery: query.NewAggregationQuery().WithAvgPath([]string{"width"}, ""),
			wantErr:          false,
			result: map[string]interface{}{
				"field_1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(4.975)}},
			},
		},
		{
			desc:             "Aggregations with limit",
			aggregationQuery: (&limitQuery).NewAggregationQuery().WithCount("count1").WithAvgPath([]string{"width"}, "width_avg1").WithSumPath([]string{"width"}, "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(4)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(12.6)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(3.15)}},
			},
		},
		{
			desc:             "Aggregations with StartAt",
			aggregationQuery: (&startAtQuery).NewAggregationQuery().WithCount("count1").WithAvgPath([]string{"width"}, "width_avg1").WithSumPath([]string{"width"}, "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(6)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(35.7)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(5.95)}},
			},
		},
		{
			desc:             "Aggregations with StartAfter",
			aggregationQuery: (&startAfterQuery).NewAggregationQuery().WithCount("count1").WithAvgPath([]string{"width"}, "width_avg1").WithSumPath([]string{"width"}, "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(5)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(32)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(6.4)}},
			},
		},
		{
			desc:             "Aggregations with EndAt",
			aggregationQuery: (&endAtQuery).NewAggregationQuery().WithCount("count1").WithAvgPath([]string{"width"}, "width_avg1").WithSumPath([]string{"width"}, "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(6)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(30.1)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(5.016666666666667)}},
			},
		},
		{
			desc:             "Aggregations with EndBefore",
			aggregationQuery: (&endBeforeQuery).NewAggregationQuery().WithCount("count1").WithAvgPath([]string{"width"}, "width_avg1").WithSumPath([]string{"width"}, "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(5)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(23)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(4.6)}},
			},
		},
		{
			desc:             "Aggregations with LimitToLast",
			aggregationQuery: (&limitToLastQuery).NewAggregationQuery().WithCount("count1").WithAvgPath([]string{"width"}, "width_avg1").WithSumPath([]string{"width"}, "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(4)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(27.2)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_DoubleValue{DoubleValue: float64(6.8)}},
			},
		},
		{
			desc:             "Aggregations on empty results",
			aggregationQuery: emptyResultsQueryPtr.NewAggregationQuery().WithCount("count1").WithAvg("width", "width_avg1").WithSum("width", "width_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"count1":     &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(0)}},
				"width_sum1": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(0)}},
				"width_avg1": &pb.Value{ValueType: &pb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}},
			},
		},
		{
			desc:             "Aggregation on non-numeric field",
			aggregationQuery: query.NewAggregationQuery().WithAvg("model", "model_avg1").WithSum("model", "model_sum1"),
			wantErr:          false,
			result: map[string]interface{}{
				"model_sum1": &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: int64(0)}},
				"model_avg1": &pb.Value{ValueType: &pb.Value_NullValue{NullValue: structpb.NullValue_NULL_VALUE}},
			},
		},
		{
			desc:             "Aggregation on non existent key",
			aggregationQuery: query.NewAggregationQuery().WithAvg("randKey", "key_avg1").WithSum("randKey", "key_sum1"),
			wantErr:          true,
		},
	}

	for _, tc := range testcases {
		var aggResult AggregationResult
		var err error
		if tc.runInTransaction {
			client.RunTransaction(ctx, func(ctx context.Context, tx *Transaction) error {
				aggResult, err = tc.aggregationQuery.Transaction(tx).Get(ctx)
				return err
			})
		} else {
			aggResult, err = tc.aggregationQuery.Get(ctx)
		}
		if err != nil && !tc.wantErr {
			t.Errorf("%s: got: %v, want: nil", tc.desc, err)
			continue
		}
		if err == nil && tc.wantErr {
			t.Errorf("%s: got: %v, wanted error", tc.desc, err)
			continue
		}
		if !reflect.DeepEqual(aggResult, tc.result) {
			t.Errorf("%s: got: %v, want: %v", tc.desc, aggResult, tc.result)
			continue
		}
	}
}

func TestIntegration_CountAggregationQuery(t *testing.T) {
	str := uid.NewSpace("firestore-count", &uid.Options{})
	datum := str.New()

	docs := []*DocumentRef{
		iColl.NewDoc(),
		iColl.NewDoc(),
	}

	c := integrationClient(t)
	ctx := context.Background()
	bw := c.BulkWriter(ctx)
	jobs := make([]*BulkWriterJob, 0)

	// Populate the collection
	f := map[string]interface{}{
		"str": datum,
	}
	for _, d := range docs {
		j, err := bw.Create(d, f)
		jobs = append(jobs, j)
		if err != nil {
			t.Fatal(err)
		}
	}
	bw.End()

	for _, j := range jobs {
		_, err := j.Results()
		if err != nil {
			t.Fatal(err)
		}
	}

	alias := "twos"
	q := iColl.Where("str", "==", datum)
	aq := q.NewAggregationQuery()
	ar, err := aq.WithCount(alias).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	count, ok := ar[alias]
	if !ok {
		t.Errorf("key %s not in response %v", alias, ar)
	}
	cv := count.(*firestorev1.Value)
	if cv.GetIntegerValue() != 2 {
		t.Errorf("COUNT aggregation query mismatch;\ngot: %d, want: %d", cv.GetIntegerValue(), 2)
	}

	t.Cleanup(func() {
		deleteDocuments(docs)
	})
}

func TestIntegration_ClientReadTime(t *testing.T) {
	docs := []*DocumentRef{
		iColl.NewDoc(),
		iColl.NewDoc(),
	}
	t.Cleanup(func() {
		deleteDocuments(docs)
	})

	c := integrationClient(t)
	ctx := context.Background()
	bw := c.BulkWriter(ctx)
	jobs := make([]*BulkWriterJob, 0)

	// Populate the collection
	f := integrationTestMap
	for _, d := range docs {
		j, err := bw.Create(d, f)
		jobs = append(jobs, j)
		if err != nil {
			t.Fatal(err)
		}
	}
	bw.End()

	for _, j := range jobs {
		_, err := j.Results()
		if err != nil {
			t.Fatal(err)
		}
	}

	tm := time.Now().Add(-time.Minute)
	c.WithReadOptions(ReadTime(tm))

	ds, err := c.GetAll(ctx, docs)
	if err != nil {
		t.Fatal(err)
	}

	wantReadTime := tm.Truncate(time.Second)
	for _, d := range ds {
		if !wantReadTime.Equal(d.ReadTime) {
			t.Errorf("wanted read time: %v; got: %v",
				tm.UnixNano(), d.ReadTime.UnixNano())
		}
	}
}
