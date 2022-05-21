package firestore

import (
	"context"
	"fmt"
	"os"
	"testing"
)

type testBulkwriterCase struct {
	DocRef    DocumentRef
	Operation bulkWriterOperation
	Value     interface{}
}

var coll *CollectionRef
var (
	collectionPath = ""
	colName        = "bulkwriter-test"
)

var (
	testCases = []testBulkwriterCase{
		{
			DocRef: DocumentRef{
				Parent: coll,
				Path:   fmt.Sprintf("%s/doc-1", collectionPath),
				ID:     "doc-1",
			},
			Operation: CREATE,
			Value: map[string]interface{}{
				"myval": 1,
			},
		},
		{
			DocRef: DocumentRef{
				Parent: coll,
				Path:   fmt.Sprintf("%s/doc-2", collectionPath),
				ID:     "doc-2",
			},
			Operation: CREATE,
			Value: map[string]interface{}{
				"myval": 2,
			},
		},
		{
			DocRef: DocumentRef{
				Parent: coll,
				Path:   fmt.Sprintf("%s/doc-3", collectionPath),
				ID:     "doc-3",
			},
			Operation: CREATE,
			Value: map[string]interface{}{
				"myval": 3,
			},
		},
		{
			DocRef: DocumentRef{
				Parent: coll,
				Path:   fmt.Sprintf("%s/doc-4", collectionPath),
				ID:     "doc-4",
			},
			Operation: CREATE,
			Value: map[string]interface{}{
				"myval": 4,
			},
		},
	}
)

func TestCallersBulkWriter(t *testing.T) {
	pid := os.Getenv("PROJECT_ID")

	ctx := context.Background()
	c, err := NewClient(ctx, pid)
	if err != nil {
		fmt.Println(fmt.Errorf("can't create new client: n%v", err))
	}

	// Set up our test cases
	coll = c.Collection(colName)
	collectionPath = fmt.Sprintf("projects/%s/databases/default/documents/%s", pid, colName)
	t.Logf("Project ID is: %s\n", pid)
	t.Logf("Collection is: %s\n", collectionPath)

	bw, err := c.BulkWriter(1)
	defer (*bw).Close()

	if err != nil {
		t.Errorf("can't create CallersBulkWriter: n%v", err)
	}

	// This is where the caller controls their go routines
	for _, tc := range testCases {
		go func(tc testBulkwriterCase) {
			res, err := (*bw).Do(&tc.DocRef, tc.Operation, tc.Value)
			if err != nil {
				fmt.Println(fmt.Errorf("error doing request: n%v", err))
			}
			fmt.Println(res)
			if res == nil {
				t.Errorf("Got a nil response")
			}
		}(tc)
	}
}
