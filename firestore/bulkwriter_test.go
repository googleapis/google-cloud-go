package firestore

import (
	"fmt"
	"testing"
)

type testBulkwriterCase struct {
	DocRef    DocumentRef
	Operation string
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
			Operation: "create",
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
			Operation: "delete",
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
			Operation: "update",
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
			Operation: "set",
			Value: map[string]interface{}{
				"myval": 4,
			},
		},
	}
)

func TestCallersBulkWriter(t *testing.T) {
	t.Log("BulkWriter unit tests not implemented")
}
