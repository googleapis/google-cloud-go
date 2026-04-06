// Copyright 2026 Google LLC
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
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func skipIfNotEnterprise(t *testing.T) {
	if testParams[firestoreEditionKey].(firestoreEdition) != editionEnterprise {
		t.Skip("Skipping test in non-enterprise environment")
	}
}

type Author struct {
	Name    string `firestore:"name"`
	Country string `firestore:"country"`
}

type Book struct {
	Title     string   `firestore:"title"`
	Author    Author   `firestore:"author"`
	Genre     string   `firestore:"genre"`
	Published int      `firestore:"published"`
	Rating    float64  `firestore:"rating"`
	Tags      []string `firestore:"tags"`
}

func testBooks() []Book {
	return []Book{
		{Title: "The Hitchhiker's Guide to the Galaxy", Author: Author{Name: "Douglas Adams", Country: "UK"}, Genre: "Science Fiction", Published: 1979, Rating: 4.2, Tags: []string{"comedy", "space", "adventure"}},
		{Title: "Pride and Prejudice", Author: Author{Name: "Jane Austen", Country: "UK"}, Genre: "Romance", Published: 1813, Rating: 4.5, Tags: []string{"classic", "social commentary", "love"}},
		{Title: "One Hundred Years of Solitude", Author: Author{Name: "Gabriel García Márquez", Country: "Colombia"}, Genre: "Magical Realism", Published: 1967, Rating: 4.3, Tags: []string{"family", "history", "fantasy"}},
		{Title: "The Lord of the Rings", Author: Author{Name: "J.R.R. Tolkien", Country: "UK"}, Genre: "Fantasy", Published: 1954, Rating: 4.7, Tags: []string{"adventure", "magic", "epic"}},
		{Title: "The Handmaid's Tale", Author: Author{Name: "Margaret Atwood", Country: "Canada"}, Genre: "Dystopian", Published: 1985, Rating: 4.1, Tags: []string{"feminism", "totalitarianism", "resistance"}},
		{Title: "Crime and Punishment", Author: Author{Name: "Fyodor Dostoevsky", Country: "Russia"}, Genre: "Psychological Thriller", Published: 1866, Rating: 4.3, Tags: []string{"philosophy", "crime", "redemption"}},
		{Title: "To Kill a Mockingbird", Author: Author{Name: "Harper Lee", Country: "USA"}, Genre: "Southern Gothic", Published: 1960, Rating: 4.2, Tags: []string{"racism", "injustice", "coming-of-age"}},
		{Title: "1984", Author: Author{Name: "George Orwell", Country: "UK"}, Genre: "Dystopian", Published: 1949, Rating: 4.2, Tags: []string{"surveillance", "totalitarianism", "propaganda"}},
		{Title: "The Great Gatsby", Author: Author{Name: "F. Scott Fitzgerald", Country: "USA"}, Genre: "Modernist", Published: 1925, Rating: 4.0, Tags: []string{"wealth", "american dream", "love"}},
		{Title: "Dune", Author: Author{Name: "Frank Herbert", Country: "USA"}, Genre: "Science Fiction", Published: 1965, Rating: 4.6, Tags: []string{"politics", "desert", "ecology"}},
	}
}

func TestIntegration_PipelineExecute(t *testing.T) {
	skipIfNotEnterprise(t)
	ctx := context.Background()
	client := integrationClient(t)
	coll := integrationColl(t)

	t.Run("WithReadOptions", func(t *testing.T) {
		timeBeforeCreate := time.Now()
		doc1 := coll.NewDoc()
		_, err := doc1.Create(ctx, map[string]interface{}{"a": 1})
		if err != nil {
			t.Fatal(err)
		}

		// Let a little time pass to ensure the next write has a later timestamp.
		time.Sleep(1 * time.Millisecond)

		doc2 := coll.NewDoc()
		_, err = doc2.Create(ctx, map[string]interface{}{"a": 2})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{doc1, doc2})
		})

		iter := client.Pipeline().Collection(coll.ID).WithReadOptions(ReadTime(timeBeforeCreate)).Execute(ctx).Results()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 0 {
			t.Errorf("got %d documents, want 0", len(res))
		}
	})
	t.Run("WithTransaction", func(t *testing.T) {
		h := testHelper{t}
		books := testBooks()[:2]
		var docRefs []*DocumentRef
		for _, b := range books {
			docRef := coll.NewDoc()
			h.mustCreate(docRef, b)
			docRefs = append(docRefs, docRef)
		}
		t.Cleanup(func() {
			deleteDocuments(docRefs)
		})
		p := client.Pipeline().Collection(coll.ID)
		err := client.RunTransaction(ctx, func(ctx context.Context, txn *Transaction) error {
			iter := txn.Execute(p).Results()
			res, err := iter.GetAll()
			if err != nil {
				return err
			}
			if len(res) != len(books) {
				return fmt.Errorf("got %d documents, want %d", len(res), len(books))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ExplainModeAnalyze", func(t *testing.T) {
		docRef := coll.NewDoc()
		h := testHelper{t}
		h.mustCreate(docRef, map[string]any{"a": 1})
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{docRef})
		})

		snap := client.Pipeline().Collection(coll.ID).
			Limit(1).
			Execute(ctx, WithExplainMode(ExplainModeAnalyze))

		_, err := snap.Results().GetAll()
		if err != nil {
			t.Fatal(err)
		}

		stats := snap.ExplainStats()
		text, err := stats.Text()
		if err != nil {
			t.Fatal(err)
		}
		if text == "" {
			t.Error("ExplainStats Text is empty")
		}
	})

	t.Run("WithRawExecuteOptions", func(t *testing.T) {
		if useEmulator {
			t.Skip("Explain with error is not supported against the emulator")
		}
		pipeline := client.Pipeline().Collection(coll.ID).Sort(Orders(Ascending(FieldOf("rating"))))
		snap := pipeline.Execute(ctx,
			WithExplainMode(ExplainModeAnalyze),
			RawOptions{"memory_limit": 1},
		)

		_, err := snap.Results().GetAll()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if status.Code(err) != codes.ResourceExhausted {
			t.Errorf("got error code %v, want %v", status.Code(err), codes.ResourceExhausted)
		}
	})

	t.Run("CreateFromQuery", func(t *testing.T) {
		h := testHelper{t}
		coll := integrationColl(t)
		books := testBooks()[:3]
		var docRefs []*DocumentRef
		for _, b := range books {
			docRef := coll.NewDoc()
			h.mustCreate(docRef, b)
			docRefs = append(docRefs, docRef)
		}
		t.Cleanup(func() {
			deleteDocuments(docRefs)
		})

		q := coll.Where("rating", ">", 4.2)
		p := client.Pipeline().CreateFromQuery(q)
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d documents, want 2", len(results))
		}
	})

	t.Run("CreateFromAggregationQuery", func(t *testing.T) {
		h := testHelper{t}
		coll := integrationColl(t)
		books := testBooks()[:3]
		var docRefs []*DocumentRef
		for _, b := range books {
			docRef := coll.NewDoc()
			h.mustCreate(docRef, b)
			docRefs = append(docRefs, docRef)
		}
		t.Cleanup(func() {
			deleteDocuments(docRefs)
		})

		ag := coll.NewAggregationQuery().WithCount("count")
		p := client.Pipeline().CreateFromAggregationQuery(ag)
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["count"] != int64(3) {
			t.Errorf("got count %d, want 3", data["count"])
		}
	})
}

func TestIntegration_PipelineStages(t *testing.T) {
	skipIfNotEnterprise(t)
	ctx := context.Background()
	client := integrationClient(t)
	coll := integrationColl(t)
	h := testHelper{t}
	type Author struct {
		Name    string `firestore:"name"`
		Country string `firestore:"country"`
	}
	type Book struct {
		Title     string   `firestore:"title"`
		Author    Author   `firestore:"author"`
		Genre     string   `firestore:"genre"`
		Published int      `firestore:"published"`
		Rating    float64  `firestore:"rating"`
		Tags      []string `firestore:"tags"`
	}
	books := []Book{
		{Title: "The Hitchhiker's Guide to the Galaxy", Author: Author{Name: "Douglas Adams", Country: "UK"}, Genre: "Science Fiction", Published: 1979, Rating: 4.2, Tags: []string{"comedy", "space", "adventure"}},
		{Title: "Pride and Prejudice", Author: Author{Name: "Jane Austen", Country: "UK"}, Genre: "Romance", Published: 1813, Rating: 4.5, Tags: []string{"classic", "social commentary", "love"}},
		{Title: "One Hundred Years of Solitude", Author: Author{Name: "Gabriel García Márquez", Country: "Colombia"}, Genre: "Magical Realism", Published: 1967, Rating: 4.3, Tags: []string{"family", "history", "fantasy"}},
		{Title: "The Lord of the Rings", Author: Author{Name: "J.R.R. Tolkien", Country: "UK"}, Genre: "Fantasy", Published: 1954, Rating: 4.7, Tags: []string{"adventure", "magic", "epic"}},
		{Title: "The Handmaid's Tale", Author: Author{Name: "Margaret Atwood", Country: "Canada"}, Genre: "Dystopian", Published: 1985, Rating: 4.1, Tags: []string{"feminism", "totalitarianism", "resistance"}},
		{Title: "Crime and Punishment", Author: Author{Name: "Fyodor Dostoevsky", Country: "Russia"}, Genre: "Psychological Thriller", Published: 1866, Rating: 4.3, Tags: []string{"philosophy", "crime", "redemption"}},
		{Title: "To Kill a Mockingbird", Author: Author{Name: "Harper Lee", Country: "USA"}, Genre: "Southern Gothic", Published: 1960, Rating: 4.2, Tags: []string{"racism", "injustice", "coming-of-age"}},
		{Title: "1984", Author: Author{Name: "George Orwell", Country: "UK"}, Genre: "Dystopian", Published: 1949, Rating: 4.2, Tags: []string{"surveillance", "totalitarianism", "propaganda"}},
		{Title: "The Great Gatsby", Author: Author{Name: "F. Scott Fitzgerald", Country: "USA"}, Genre: "Modernist", Published: 1925, Rating: 4.0, Tags: []string{"wealth", "american dream", "love"}},
		{Title: "Dune", Author: Author{Name: "Frank Herbert", Country: "USA"}, Genre: "Science Fiction", Published: 1965, Rating: 4.6, Tags: []string{"politics", "desert", "ecology"}},
	}
	var docRefs []*DocumentRef
	for _, b := range books {
		docRef := coll.NewDoc()
		h.mustCreate(docRef, b)
		docRefs = append(docRefs, docRef)
	}
	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})
	t.Run("AddFields", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).AddFields(Selectables(Multiply(FieldOf("rating"), 2).As("doubled_rating"))).Limit(1).Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if dr, ok := data["doubled_rating"]; !ok || dr.(float64) != data["rating"].(float64)*2 {
			t.Errorf("got doubled_rating %v, want %v", dr, data["rating"].(float64)*2)
		}
	})
	t.Run("Aggregate", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Aggregate(Accumulators(Count("rating").As("total_books"))).Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}

		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["total_books"] != int64(10) {
			t.Errorf("got %d total_books, want 10", data["total_books"])
		}
	})
	t.Run("AggregateWith", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Aggregate(Accumulators(Average("rating").As("avg_rating")), WithAggregateGroups("genre")).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 8 {
			t.Errorf("got %d groups, want 8", len(results))
		}
	})
	t.Run("Distinct", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Distinct(Fields("genre")).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 8 {
			t.Errorf("got %d distinct genres, want 8", len(results))
		}
	})
	t.Run("Documents", func(t *testing.T) {
		iter := client.Pipeline().Documents([]*DocumentRef{docRefs[0], docRefs[1]}).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d documents, want 2", len(results))
		}
	})
	t.Run("CollectionGroup", func(t *testing.T) {
		cgCollID := collectionIDs.New()
		doc1 := coll.Doc("cg_doc1")
		doc2 := coll.Doc("cg_doc2")
		cgColl1 := doc1.Collection(cgCollID)
		cgColl2 := doc2.Collection(cgCollID)
		cgDoc1 := cgColl1.NewDoc()
		cgDoc2 := cgColl2.NewDoc()
		h.mustCreate(cgDoc1, map[string]string{"val": "a"})
		h.mustCreate(cgDoc2, map[string]string{"val": "b"})
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{cgDoc1, cgDoc2, doc1, doc2})
		})
		iter := client.Pipeline().CollectionGroup(cgCollID).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d documents, want 2", len(results))
		}
	})
	t.Run("CollectionHints", func(t *testing.T) {
		docRef := coll.NewDoc()
		h.mustCreate(docRef, map[string]any{"a": 1})
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{docRef})
		})

		// Use a hint that is likely ignored or causes no error if valid.
		client.Pipeline().Collection(coll.ID, WithIgnoreIndexFields("a")).
			Where(Equal("a", 1)).
			Execute(ctx).Results()
	})
	t.Run("Database", func(t *testing.T) {
		dbDoc1 := coll.Doc("db_doc1")
		otherColl := client.Collection(collectionIDs.New())
		dbDoc2 := otherColl.Doc("db_doc2")
		h.mustCreate(dbDoc1, map[string]string{"val": "a"})
		h.mustCreate(dbDoc2, map[string]string{"val": "b"})
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{dbDoc1, dbDoc2})
		})
		iter := client.Pipeline().Database().Limit(2).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d documents, want 2", len(results))
		}
	})
	t.Run("Literals", func(t *testing.T) {
		iter := client.Pipeline().Literals([]map[string]any{
			map[string]any{"name": "joe", "age": 10},
			map[string]any{"name": "bob", "age": 30},
			map[string]any{"name": "alice", "age": 40},
		}).
			Where(GreaterThan(FieldOf("age"), 20)).
			Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d documents, want 2", len(results))
		}
	})
	t.Run("Constants", func(t *testing.T) {
		iter := client.Pipeline().Literals([]map[string]any{map[string]any{"a": 1}}).
			Select(Fields(
				ConstantOfNull().As("null"),
				ConstantOfVector32([]float32{1.5, 2.5, 3.5}).As("v32"),
				ConstantOfVector64([]float64{4.5, 5.5, 6.5}).As("v64"),
			)).
			Execute(ctx).Results()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("got %d docs, want 1", len(res))
		}
		data := res[0].Data()
		if data["null"] != nil {
			t.Errorf("got %v, want nil", data["null"])
		}
		if v32, ok := data["v32"].(Vector64); !ok || len(v32) != 3 || v32[0] != 1.5 {
			t.Errorf("got v32 %v (type %T)", data["v32"], data["v32"])
		}
		if v64, ok := data["v64"].(Vector64); !ok || len(v64) != 3 || v64[0] != 4.5 {
			t.Errorf("got v64 %v (type %T)", data["v64"], data["v64"])
		}
	})
	t.Run("FindNearest", func(t *testing.T) {
		type DocWithVector struct {
			ID     string   `firestore:"id"`
			Vector Vector32 `firestore:"vector"`
		}
		docsWithVector := []DocWithVector{
			{ID: "doc1", Vector: Vector32{1.0, 2.0, 3.0}},
			{ID: "doc2", Vector: Vector32{4.0, 5.0, 6.0}},
			{ID: "doc3", Vector: Vector32{7.0, 8.0, 9.0}},
		}
		var vectorDocRefs []*DocumentRef
		for _, d := range docsWithVector {
			docRef := coll.NewDoc()
			h.mustCreate(docRef, d)
			vectorDocRefs = append(vectorDocRefs, docRef)
		}
		t.Cleanup(func() {
			deleteDocuments(vectorDocRefs)
		})
		queryVector := Vector32{1.1, 2.1, 3.1}
		iter := client.Pipeline().Collection(coll.ID).
			FindNearest("vector", queryVector, PipelineDistanceMeasureEuclidean, RawOptions{"limit": 2, "distance_field": "distance"}).
			Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("got %d documents, want 2", len(results))
		}
		// Check if the results are sorted by distance

		if !results[0].Exists() {
			t.Fatalf("results[0] Exists: got: false, want: true")
		}
		dist1 := results[0].Data()

		if !results[1].Exists() {
			t.Fatalf("results[1] Exists: got: false, want: true")
		}
		dist2 := results[1].Data()
		if dist1["distance"].(float64) > dist2["distance"].(float64) {
			t.Errorf("documents are not sorted by distance")
		}
		// Check if the correct documents are returned
		if dist1["id"] != "doc1" {
			t.Errorf("got doc id %q, want 'doc1'", dist1["id"])
		}
	})
	t.Run("Limit", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Limit(3).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("got %d documents, want 3", len(results))
		}
	})
	t.Run("Offset", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Sort(Orders(Ascending(FieldOf("published")))).Offset(2).Limit(1).Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["title"] != "The Great Gatsby" {
			t.Errorf("got title %q, want 'The Great Gatsby'", data["title"])
		}
	})
	t.Run("RawStage", func(t *testing.T) {
		// Using RawStage to perform a Limit operation
		iter := client.Pipeline().Collection(coll.ID).RawStage("limit", []any{3}).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("got %d documents, want 3", len(results))
		}

		// Using RawStage to perform a Select operation with options
		iter = client.Pipeline().Collection(coll.ID).RawStage("select", []any{map[string]interface{}{"title": FieldOf("title")}}).Limit(1).Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if _, ok := data["title"]; !ok {
			t.Error("missing 'title' field")
		}
		if _, ok := data["genre"]; ok {
			t.Error("unexpected 'genre' field")
		}
	})
	t.Run("RemoveFields", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).
			Limit(1).
			RemoveFields(Fields("genre", "rating")).
			Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if _, ok := data["genre"]; ok {
			t.Error("unexpected 'genre' field")
		}
		if _, ok := data["rating"]; ok {
			t.Error("unexpected 'rating' field")
		}
		if _, ok := data["title"]; !ok {
			t.Error("missing 'title' field")
		}
	})
	t.Run("Replace", func(t *testing.T) {
		type DocWithMap struct {
			ID   string         `firestore:"id"`
			Data map[string]int `firestore:"data"`
		}
		docWithMap := DocWithMap{ID: "docWithMap", Data: map[string]int{"a": 1, "b": 2}}
		docRef := coll.NewDoc()
		h.mustCreate(docRef, docWithMap)
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{docRef})
		})
		iter := client.Pipeline().Collection(coll.ID).
			Where(Equal(FieldOf("id"), "docWithMap")).
			ReplaceWith("data").
			Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		want := map[string]interface{}{"a": int64(1), "b": int64(2)}
		if diff := testutil.Diff(data, want); diff != "" {
			t.Errorf("got: %v, want: %v, diff +want -got: %s", data, want, diff)
		}
	})
	t.Run("Sample", func(t *testing.T) {
		t.Run("SampleWithDocLimit", func(t *testing.T) {
			iter := client.Pipeline().Collection(coll.ID).Sample(WithDocLimit(5)).Execute(ctx).Results()
			defer iter.Stop()
			var got []map[string]interface{}
			for {
				doc, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Fatalf("Failed to iterate: %v", err)
				}
				if !doc.Exists() {
					t.Fatalf("Exists: got: false, want: true")
				}
				data := doc.Data()
				got = append(got, data)
			}
			if len(got) != 5 {
				t.Errorf("got %d documents, want 5", len(got))
			}
		})
		t.Run("SampleWithPercentage", func(t *testing.T) {
			iter := client.Pipeline().Collection(coll.ID).Sample(WithPercentage(0.6)).Execute(ctx).Results()
			defer iter.Stop()
			var got []map[string]interface{}
			for {
				doc, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Fatalf("Failed to iterate: %v", err)
				}
				if !doc.Exists() {
					t.Fatalf("Exists: got: false, want: true")
				}
				data := doc.Data()
				got = append(got, data)
			}
			if len(got) >= 10 {
				t.Errorf("Sampled documents count should be less than total. got %d, total 10", len(got))
			}
			if len(got) == 0 {
				t.Errorf("Sampled documents count should be greater than 0. got %d", len(got))
			}
		})
	})
	t.Run("Select", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Select(Fields("title", "author.name")).Limit(1).Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if _, ok := data["title"]; !ok {
			t.Error("missing 'title' field")
		}

		authorRaw, ok := data["author"]
		if !ok {
			t.Error("missing 'author' map from backend reconstructed field path")
		} else if authorMap, ok := authorRaw.(map[string]interface{}); !ok {
			t.Errorf("'author' is not a map, got %T", authorRaw)
		} else if _, ok := authorMap["name"]; !ok {
			t.Error("missing nested 'name' field inside author map")
		}

		if _, ok := data["genre"]; ok {
			t.Error("unexpected 'genre' field")
		}
	})
	t.Run("Sort", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Sort(Orders(Descending(FieldOf("rating")))).Limit(1).Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["title"] != "The Lord of the Rings" {
			t.Errorf("got title %q, want 'The Lord of the Rings'", data["title"])
		}
	})
	t.Run("Union", func(t *testing.T) {
		type Employee struct {
			Name string `firestore:"name"`
			Age  int    `firestore:"age"`
		}
		type Customer struct {
			Name    string `firestore:"name"`
			Address string `firestore:"address"`
		}
		employeeColl := client.Collection(collectionIDs.New())
		customerColl := client.Collection(collectionIDs.New())
		employees := []Employee{
			{Name: "John Doe", Age: 42},
			{Name: "Jane Smith", Age: 35},
		}
		customers := []Customer{
			{Name: "Alice", Address: "123 Main St"},
			{Name: "Bob", Address: "456 Oak Ave"},
		}
		var unionDocRefs []*DocumentRef
		for _, e := range employees {
			docRef := employeeColl.NewDoc()
			h.mustCreate(docRef, e)
			unionDocRefs = append(unionDocRefs, docRef)
		}
		for _, c := range customers {
			docRef := customerColl.NewDoc()
			h.mustCreate(docRef, c)
			unionDocRefs = append(unionDocRefs, docRef)
		}
		t.Cleanup(func() {
			deleteDocuments(unionDocRefs)
		})
		employeePipeline := client.Pipeline().Collection(employeeColl.ID)
		customerPipeline := client.Pipeline().Collection(customerColl.ID)
		iter := employeePipeline.Union(customerPipeline).Execute(context.Background()).Results()
		defer iter.Stop()
		var got []map[string]interface{}
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("Failed to iterate: %v", err)
			}
			if !doc.Exists() {
				t.Fatalf("Exists: got: false, want: true")
			}
			data := doc.Data()
			got = append(got, data)
		}
		want := []map[string]interface{}{
			{"name": "John Doe", "age": int64(42)},
			{"name": "Jane Smith", "age": int64(35)},
			{"name": "Alice", "address": "123 Main St"},
			{"name": "Bob", "address": "456 Oak Ave"},
		}
		sort.Slice(got, func(i, j int) bool {
			return got[i]["name"].(string) < got[j]["name"].(string)
		})
		sort.Slice(want, func(i, j int) bool {
			return want[i]["name"].(string) < want[j]["name"].(string)
		})
		if diff := testutil.Diff(got, want); diff != "" {
			t.Errorf("got: %v, want: %v, diff +want -got: %s", got, want, diff)
		}
	})
	t.Run("Unnest", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).
			Where(Equal(FieldOf("title"), "The Hitchhiker's Guide to the Galaxy")).
			UnnestWithAlias("tags", "tag", nil).
			Select(Fields("title", "tag")).
			Execute(ctx).Results()
		defer iter.Stop()
		var got []map[string]interface{}
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("Failed to iterate: %v", err)
			}
			if !doc.Exists() {
				t.Fatalf("Exists: got: false, want: true")
			}
			data := doc.Data()
			got = append(got, data)
		}
		want := []map[string]interface{}{
			{"title": "The Hitchhiker's Guide to the Galaxy", "tag": "comedy"},
			{"title": "The Hitchhiker's Guide to the Galaxy", "tag": "space"},
			{"title": "The Hitchhiker's Guide to the Galaxy", "tag": "adventure"},
		}
		sort.Slice(got, func(i, j int) bool {
			return got[i]["tag"].(string) < got[j]["tag"].(string)
		})
		sort.Slice(want, func(i, j int) bool {
			return want[i]["tag"].(string) < want[j]["tag"].(string)
		})
		if diff := testutil.Diff(got, want); diff != "" {
			t.Errorf("got: %v, want: %v, diff +want -got: %s", got, want, diff)
		}
	})
	t.Run("UnnestWithFieldOf", func(t *testing.T) {
		docRef := coll.NewDoc()
		h.mustCreate(docRef, map[string]any{
			"tags": []string{"a", "b", "c"},
		})
		t.Cleanup(func() {
			deleteDocuments([]*DocumentRef{docRef})
		})

		iter := client.Pipeline().Collection(coll.ID).
			Where(Equal(FieldOf("__name__"), docRef)).
			Unnest(FieldOf("tags").As("tag"), nil).
			Execute(ctx).Results()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 3 {
			t.Fatalf("got %d docs, want 3", len(res))
		}
	})
	t.Run("UnnestWithIndexField", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).
			Where(Equal(FieldOf("title"), "The Hitchhiker's Guide to the Galaxy")).
			UnnestWithAlias("tags", "tag", WithUnnestIndexField("tagIndex")).
			Select(Fields("title", "tag", "tagIndex")).
			Execute(ctx).Results()
		defer iter.Stop()
		var got []map[string]interface{}
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatalf("Failed to iterate: %v", err)
			}
			if !doc.Exists() {
				t.Fatalf("Exists: got: false, want: true")
			}
			data := doc.Data()
			got = append(got, data)
		}
		want := []map[string]interface{}{
			{"title": "The Hitchhiker's Guide to the Galaxy", "tag": "comedy", "tagIndex": int64(0)},
			{"title": "The Hitchhiker's Guide to the Galaxy", "tag": "space", "tagIndex": int64(1)},
			{"title": "The Hitchhiker's Guide to the Galaxy", "tag": "adventure", "tagIndex": int64(2)},
		}
		sort.Slice(got, func(i, j int) bool {
			return got[i]["tagIndex"].(int64) < got[j]["tagIndex"].(int64)
		})
		if diff := testutil.Diff(got, want); diff != "" {
			t.Errorf("got: %v, want: %v, diff +want -got: %s", got, want, diff)
		}
	})
	t.Run("Where", func(t *testing.T) {
		iter := client.Pipeline().Collection(coll.ID).Where(Equal(FieldOf("author.country"), "UK")).Execute(ctx).Results()
		defer iter.Stop()
		results, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(results) != 4 {
			t.Errorf("got %d documents, want 4", len(results))
		}
	})
	t.Run("Update", func(t *testing.T) {
		t.Skip("Skipping test until feature is available in PROD")
		updateIter := client.Pipeline().Collection(coll.ID).
			Where(Equal(FieldOf("author.country"), "UK")).
			Update(WithUpdateTransformations(ConstantOf("Active").As("status"))).
			Execute(ctx).Results()
		defer updateIter.Stop()
		_, err := updateIter.GetAll()
		if err != nil {
			t.Fatalf("Failed to execute update: %v", err)
		}

		verifyIter := client.Pipeline().Collection(coll.ID).Where(Equal(FieldOf("status"), "Active")).Execute(ctx).Results()
		defer verifyIter.Stop()
		results, err := verifyIter.GetAll()
		if err != nil {
			t.Fatalf("Failed to execute verify: %v", err)
		}
		if len(results) != 4 {
			t.Errorf("got %d updated documents, want 4", len(results))
		}
	})
	t.Run("Delete", func(t *testing.T) {
		t.Skip("Skipping test until feature is available in PROD")
		deleteIter := client.Pipeline().Collection(coll.ID).Where(Equal(FieldOf("title"), "The Great Gatsby")).Delete().Execute(ctx).Results()
		defer deleteIter.Stop()
		_, err := deleteIter.GetAll()
		if err != nil {
			t.Fatalf("Failed to execute delete: %v", err)
		}

		verifyIter := client.Pipeline().Collection(coll.ID).Where(Equal(FieldOf("title"), "The Great Gatsby")).Execute(ctx).Results()
		defer verifyIter.Stop()
		results, err := verifyIter.GetAll()
		if err != nil {
			t.Fatalf("Failed to execute verify: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("got %d documents, want 0 after delete", len(results))
		}
	})
}

func TestIntegration_PipelineFunctions(t *testing.T) {
	skipIfNotEnterprise(t)
	t.Run("arrayFuncs", arrayFuncs)
	t.Run("stringFuncs", stringFuncs)
	t.Run("vectorFuncs", vectorFuncs)

	t.Run("timestampFuncs", timestampFuncs)
	t.Run("arithmeticFuncs", arithmeticFuncs)
	t.Run("aggregateFuncs", aggregateFuncs)
	t.Run("comparisonFuncs", comparisonFuncs)
	t.Run("generalFuncs", generalFuncs)
	t.Run("keyFuncs", keyFuncs)
	t.Run("objectFuncs", objectFuncs)
	t.Run("logicalFuncs", logicalFuncs)
	t.Run("typeFuncs", typeFuncs)
	t.Run("aggregationFuncs", aggregationFuncs)
}

func aggregationFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())

	docData := []struct {
		Category string   `firestore:"category"`
		Val      int      `firestore:"val"`
		Tags     []string `firestore:"tags"`
	}{
		{Category: "A", Val: 1, Tags: []string{"x"}},
		{Category: "A", Val: 2, Tags: []string{"x", "y"}},
		{Category: "A", Val: 1, Tags: []string{"z"}},
	}

	var docRefs []*DocumentRef
	for _, d := range docData {
		docRef := coll.NewDoc()
		h.mustCreate(docRef, d)
		docRefs = append(docRefs, docRef)
	}
	defer deleteDocuments(docRefs)

	pipeline := client.Pipeline().Collection(coll.ID).
		Sort(Orders(Ascending(FieldOf("val")))).
		Aggregate(Accumulators(
			First("val").As("first_val"),
			Last("val").As("last_val"),
			ArrayAgg("val").As("all_vals"),
			ArrayAggDistinct("val").As("distinct_vals"),
			CountDistinct("val").As("distinct_count_val"),
			ArrayAgg("tags").As("all_tags"),
		))

	iter := pipeline.Execute(context.Background()).Results()
	defer iter.Stop()

	res, err := iter.Next()
	if err != nil {
		t.Fatalf("iter.Next() failed: %v", err)
	}

	data := res.Data()

	// Check ArrayAgg "all_vals" -> [1, 2, 1] (order irrelevant)
	allValsRaw, ok := data["all_vals"].([]interface{})
	if !ok {
		t.Fatalf("all_vals is not []interface{}, got %T", data["all_vals"])
	}
	allVals := make([]int, len(allValsRaw))
	for i, v := range allValsRaw {
		allVals[i] = int(v.(int64))
	}
	sort.Ints(allVals)
	if diff := testutil.Diff(allVals, []int{1, 1, 2}); diff != "" {
		t.Errorf("all_vals mismatch: %s", diff)
	}

	// Check ArrayAggDistinct "distinct_vals" -> [1, 2]
	distinctValsRaw, ok := data["distinct_vals"].([]interface{})
	if !ok {
		t.Fatalf("distinct_vals is not []interface{}, got %T", data["distinct_vals"])
	}
	distinctVals := make([]int, len(distinctValsRaw))
	for i, v := range distinctValsRaw {
		distinctVals[i] = int(v.(int64))
	}
	sort.Ints(distinctVals)
	if diff := testutil.Diff(distinctVals, []int{1, 2}); diff != "" {
		t.Errorf("distinct_vals mismatch: %s", diff)
	}

	// Check CountDistinct "distinct_count_val" -> 2
	if data["distinct_count_val"] != int64(2) {
		t.Errorf("got distinct_count_val %d, want 2", data["distinct_count_val"])
	}

	if _, ok := data["first_val"]; !ok {
		t.Error("first_val missing")
	}
	if _, ok := data["last_val"]; !ok {
		t.Error("last_val missing")
	}
}

func typeFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"a": nil,
		"b": true,
		"c": 1,
		"d": "hello",
		"e": []byte("world"),
		"f": time.Now(),
		"g": &latlng.LatLng{Latitude: 32.1, Longitude: -4.5},
		"h": []interface{}{1, 2, 3},
		"i": map[string]interface{}{"j": 1},
		"k": Vector64{1, 2, 3},
		"l": docRef1,
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "Type of null",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("a").As("type"))),
			want:     map[string]interface{}{"type": "null"},
		},
		{
			name:     "Type of boolean",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("b").As("type"))),
			want:     map[string]interface{}{"type": "boolean"},
		},
		{
			name:     "Type of int64",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("c").As("type"))),
			want:     map[string]interface{}{"type": "int64"},
		},
		{
			name:     "Type of string",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("d").As("type"))),
			want:     map[string]interface{}{"type": "string"},
		},
		{
			name:     "Type of bytes",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("e").As("type"))),
			want:     map[string]interface{}{"type": "bytes"},
		},
		{
			name:     "Type of timestamp",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("f").As("type"))),
			want:     map[string]interface{}{"type": "timestamp"},
		},
		{
			name:     "Type of geopoint",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("g").As("type"))),
			want:     map[string]interface{}{"type": "geo_point"},
		},
		{
			name:     "Type of array",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("h").As("type"))),
			want:     map[string]interface{}{"type": "array"},
		},
		{
			name:     "Type of map",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("i").As("type"))),
			want:     map[string]interface{}{"type": "map"},
		},
		{
			name:     "Type of vector",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("k").As("type"))),
			want:     map[string]interface{}{"type": "vector"},
		},
		{
			name:     "Type of reference",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Type("l").As("type"))),
			want:     map[string]interface{}{"type": "reference"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()
			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}
}

func TestIntegration_Query_Pipeline(t *testing.T) {
	skipIfNotEnterprise(t)
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	type Book struct {
		Title     string  `firestore:"title"`
		Genre     string  `firestore:"genre"`
		Published int     `firestore:"published"`
		Rating    float64 `firestore:"rating"`
	}
	books := []Book{
		{Title: "The Hitchhiker's Guide to the Galaxy", Genre: "Science Fiction", Published: 1979, Rating: 4.2},
		{Title: "Pride and Prejudice", Genre: "Romance", Published: 1813, Rating: 4.5},
		{Title: "One Hundred Years of Solitude", Genre: "Magical Realism", Published: 1967, Rating: 4.3},
	}
	var docRefs []*DocumentRef
	for _, b := range books {
		docRef := coll.NewDoc()
		h.mustCreate(docRef, b)
		docRefs = append(docRefs, docRef)
	}
	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})

	t.Run("Where", func(t *testing.T) {
		q := coll.Where("published", ">", 1900)
		p := q.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(res) != 2 {
			t.Errorf("got %d documents, want 2", len(res))
		}
	})

	t.Run("OrderBy", func(t *testing.T) {
		q := coll.OrderBy("published", Asc)
		p := q.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(res) != 3 {
			t.Errorf("got %d documents, want 3", len(res))
		}
		var publishedYears []int64
		for _, r := range res {
			publishedYears = append(publishedYears, r.Data()["published"].(int64))
		}
		if !sort.SliceIsSorted(publishedYears, func(i, j int) bool { return publishedYears[i] < publishedYears[j] }) {
			t.Errorf("results not sorted by published year: %v", publishedYears)
		}
	})

	t.Run("Limit", func(t *testing.T) {
		q := coll.Limit(2)
		p := q.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(res) != 2 {
			t.Errorf("got %d documents, want 2", len(res))
		}
	})

	t.Run("Offset", func(t *testing.T) {
		q := coll.OrderBy("published", Asc).Offset(1)
		p := q.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		res, err := iter.GetAll()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		if len(res) != 2 {
			t.Errorf("got %d documents, want 2", len(res))
		}
	})

	t.Run("Select", func(t *testing.T) {
		q := coll.Select("title")
		p := q.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}
		data := doc.Data()
		if _, ok := data["title"]; !ok {
			t.Error("missing 'title' field")
		}
		if _, ok := data["genre"]; ok {
			t.Error("unexpected 'genre' field")
		}
	})
}

func TestIntegration_AggregationQuery_Pipeline(t *testing.T) {
	skipIfNotEnterprise(t)
	ctx := context.Background()
	coll := integrationColl(t)
	h := testHelper{t}
	type Book struct {
		Title     string  `firestore:"title"`
		Genre     string  `firestore:"genre"`
		Published int     `firestore:"published"`
		Rating    float64 `firestore:"rating"`
	}
	books := []Book{
		{Title: "The Hitchhiker's Guide to the Galaxy", Genre: "Science Fiction", Published: 1979, Rating: 4.2},
		{Title: "Pride and Prejudice", Genre: "Romance", Published: 1813, Rating: 4.5},
		{Title: "One Hundred Years of Solitude", Genre: "Magical Realism", Published: 1967, Rating: 4.3},
	}
	var docRefs []*DocumentRef
	for _, b := range books {
		docRef := coll.NewDoc()
		h.mustCreate(docRef, b)
		docRefs = append(docRefs, docRef)
	}
	t.Cleanup(func() {
		deleteDocuments(docRefs)
	})

	t.Run("Count", func(t *testing.T) {
		ag := coll.NewAggregationQuery().WithCount("count")
		p := ag.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}

		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["count"] != int64(3) {
			t.Errorf("got %d count, want 3", data["count"])
		}
	})

	t.Run("Sum", func(t *testing.T) {
		ag := coll.NewAggregationQuery().WithSum("published", "total_published")
		p := ag.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}

		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["total_published"] != int64(1979+1813+1967) {
			t.Errorf("got %d total_published, want %d", data["total_published"], int64(1979+1813+1967))
		}
	})

	t.Run("Average", func(t *testing.T) {
		ag := coll.NewAggregationQuery().WithAvg("rating", "avg_rating")
		p := ag.Pipeline()
		iter := p.Execute(ctx).Results()
		defer iter.Stop()
		doc, err := iter.Next()
		if err != nil {
			t.Fatalf("Failed to iterate: %v", err)
		}

		if !doc.Exists() {
			t.Fatalf("Exists: got: false, want: true")
		}
		data := doc.Data()
		if data["avg_rating"] != (4.2+4.5+4.3)/3 {
			t.Errorf("got %f avg_rating, want %f", data["avg_rating"], (4.2+4.5+4.3)/3)
		}
	})
}

func objectFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"m1": map[string]interface{}{"a": 1, "b": 2},
		"m2": map[string]interface{}{"c": 3, "d": 4},
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "Map",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Map(map[string]any{"a": 1, "b": 2}).As("map"))),
			want:     map[string]interface{}{"map": map[string]interface{}{"a": int64(1), "b": int64(2)}},
		},
		{
			name:     "MapGet",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapGet("m1", "a").As("value"))),
			want:     map[string]interface{}{"value": int64(1)},
		},
		{
			name:     "MapMerge",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapMerge("m1", FieldOf("m2")).As("merged"))),
			want:     map[string]interface{}{"merged": map[string]interface{}{"a": int64(1), "b": int64(2), "c": int64(3), "d": int64(4)}},
		},
		{
			name:     "MapRemove",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapRemove("m1", "a").As("removed"))),
			want:     map[string]interface{}{"removed": map[string]interface{}{"b": int64(2)}},
		},
		{
			name:     "MapSet",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapSet("m1", "c", 3).As("updated"))),
			want:     map[string]interface{}{"updated": map[string]interface{}{"a": int64(1), "b": int64(2), "c": int64(3)}},
		},
		{
			name:     "MapKeys",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapKeys("m1").As("keys"))),
			want:     map[string]interface{}{"keys": []interface{}{"a", "b"}},
		},
		{
			name:     "MapValues",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapValues("m1").As("values"))),
			want:     map[string]interface{}{"values": []interface{}{int64(1), int64(2)}},
		},
		{
			name:     "MapEntries",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(MapEntries("m1").As("entries"))),
			want:     map[string]interface{}{"entries": []interface{}{map[string]interface{}{"k": "a", "v": int64(1)}, map[string]interface{}{"k": "b", "v": int64(2)}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()
			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}
}

func arrayFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"a":      []interface{}{1, 2, 3},
		"b":      []interface{}{4, 5, 6},
		"tags":   []string{"Go", "Firestore", "GCP"},
		"tags2":  []string{"Go", "Firestore"},
		"lang":   "Go",
		"status": "active",
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "ArrayLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayLength("a").As("length"))),
			want:     map[string]interface{}{"length": int64(3)},
		},
		{
			name:     "Array",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Array(1, 2, 3).As("array"))),
			want:     map[string]interface{}{"array": []interface{}{int64(1), int64(2), int64(3)}},
		},
		{
			name:     "ArrayFromSlice",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayFromSlice([]int{1, 2, 3}).As("array"))),
			want:     map[string]interface{}{"array": []interface{}{int64(1), int64(2), int64(3)}},
		},
		{
			name:     "ArrayGet",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayGet("a", 1).As("element"))),
			want:     map[string]interface{}{"element": int64(2)},
		},
		{
			name:     "ArrayReverse",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayReverse("a").As("reversed"))),
			want:     map[string]interface{}{"reversed": []interface{}{int64(3), int64(2), int64(1)}},
		},
		{
			name:     "ArrayConcat",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayConcat("a", FieldOf("b")).As("concatenated"))),
			want:     map[string]interface{}{"concatenated": []interface{}{int64(1), int64(2), int64(3), int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArraySum",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArraySum("a").As("sum"))),
			want:     map[string]interface{}{"sum": int64(6)},
		},
		{
			name:     "ArrayMaximum",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayMaximum("a").As("max"))),
			want:     map[string]interface{}{"max": int64(3)},
		},
		{
			name:     "ArrayMinimum",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayMinimum("a").As("min"))),
			want:     map[string]interface{}{"min": int64(1)},
		},
		{
			name:     "ArrayMaximumN",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayMaximumN("a", 2).As("max_n"))),
			want:     map[string]interface{}{"max_n": []interface{}{int64(3), int64(2)}},
		},
		{
			name:     "ArrayMinimumN",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayMinimumN("a", 2).As("min_n"))),
			want:     map[string]interface{}{"min_n": []interface{}{int64(1), int64(2)}},
		},
		{
			name:     "ArrayFirst",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayFirst("a").As("first"))),
			want:     map[string]interface{}{"first": int64(1)},
		},
		{
			name:     "ArrayFirstN",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayFirstN("a", 2).As("first_n"))),
			want:     map[string]interface{}{"first_n": []interface{}{int64(1), int64(2)}},
		},
		{
			name:     "ArrayLast",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayLast("a").As("last"))),
			want:     map[string]interface{}{"last": int64(3)},
		},
		{
			name:     "ArrayLastN",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayLastN("a", 2).As("last_n"))),
			want:     map[string]interface{}{"last_n": []interface{}{int64(2), int64(3)}},
		},
		{
			name:     "ArraySlice",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArraySlice("a", 1).As("slice"))),
			want:     map[string]interface{}{"slice": []interface{}{int64(2), int64(3)}},
		},
		{
			name:     "ArraySliceWithLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArraySliceLength("a", 1, 1).As("slice_len"))),
			want:     map[string]interface{}{"slice_len": []interface{}{int64(2)}},
		},
		// TODO: Uncomment this after fixing the proto representation of this function.
		// {
		// 	name:     "ArrayFilter",
		// 	pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayFilter("a", "x", GreaterThan(FieldOf("x"), int64(1))).As("filter"))),
		// 	want:     map[string]interface{}{"filter": []interface{}{int64(2), int64(3)}},
		// },
		{
			name:     "ArrayIndexOf",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayIndexOf("a", 2).As("index"))),
			want:     map[string]interface{}{"index": int64(1)},
		},
		{
			name:     "ArrayIndexOfAll",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayIndexOfAll(Array(1, 2, 1), 1).As("indices"))),
			want:     map[string]interface{}{"indices": []interface{}{int64(0), int64(2)}},
		},
		{
			name:     "ArrayLastIndexOf",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ArrayLastIndexOf(Array(1, 2, 1), 1).As("lastIndex"))),
			want:     map[string]interface{}{"lastIndex": int64(2)},
		},
		// Array filter conditions
		{
			name:     "ArrayContains",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContains("tags", "Go")),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAll - array of mixed types",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAll("tags", []any{FieldOf("lang"), "Firestore"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAll - array of constants",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAll("tags", []string{"Go", "Firestore"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAll - Expr",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAll("tags", FieldOf("tags2"))),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAny",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAny("tags", []string{"Go", "Java"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "EqualAny",
			pipeline: client.Pipeline().Collection(coll.ID).Where(EqualAny("status", []string{"active", "pending"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "NotEqualAny",
			pipeline: client.Pipeline().Collection(coll.ID).Where(NotEqualAny("status", []string{"archived", "deleted"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Retry(t, 3, time.Second, func(r *testutil.R) {
				ctx := context.Background()
				iter := test.pipeline.Execute(ctx).Results()
				defer iter.Stop()

				docs, err := iter.GetAll()
				if err != nil {
					t.Fatalf("GetAll: %v", err)
					return
				}
				if len(docs) != 1 {
					t.Fatalf("expected 1 doc, got %d", len(docs))
					return
				}
				got := docs[0].Data()
				if diff := testutil.Diff(got, test.want); diff != "" {
					t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
					return
				}
			})
		})
	}
}

func stringFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"name":        "  John Doe  ",
		"description": "This is a Firestore document.",
		"productCode": "abc-123",
		"tags":        []string{"tag1", "tag2", "tag3"},
		"email":       "john.doe@example.com",
		"zipCode":     "12345",
		"csv":         "a,b,c",
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	doc1want := map[string]interface{}{
		"name":        "  John Doe  ",
		"description": "This is a Firestore document.",
		"productCode": "abc-123",
		"tags":        []interface{}{"tag1", "tag2", "tag3"},
		"email":       "john.doe@example.com",
		"zipCode":     "12345",
		"csv":         "a,b,c",
	}

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     interface{}
	}{
		{
			name:     "ByteLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ByteLength("name").As("byte_length"))),
			want:     map[string]interface{}{"byte_length": int64(12)},
		},
		{
			name:     "CharLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(CharLength("name").As("char_length"))),
			want:     map[string]interface{}{"char_length": int64(12)},
		},
		{
			name:     "StringConcat",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(StringConcat(FieldOf("name"), " - ", FieldOf("productCode")).As("concatenated_string"))),
			want:     map[string]interface{}{"concatenated_string": "  John Doe   - abc-123"},
		},
		{
			name:     "StringReverse",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(StringReverse("name").As("reversed_string"))),
			want:     map[string]interface{}{"reversed_string": "  eoD nhoJ  "},
		},
		{
			name:     "Join",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Join("tags", ", ").As("joined_string"))),
			want:     map[string]interface{}{"joined_string": "tag1, tag2, tag3"},
		},
		{
			name:     "Substring",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Substring("description", 0, 4).As("substring"))),
			want:     map[string]interface{}{"substring": "This"},
		},
		{
			name:     "ToLower",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ToLower("name").As("lowercase_name"))),
			want:     map[string]interface{}{"lowercase_name": "  john doe  "},
		},
		{
			name:     "ToUpper",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(ToUpper("name").As("uppercase_name"))),
			want:     map[string]interface{}{"uppercase_name": "  JOHN DOE  "},
		},
		{
			name:     "Trim",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Trim("name").As("trimmed_name"))),
			want:     map[string]interface{}{"trimmed_name": "John Doe"},
		},
		{
			name:     "TrimValue",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(TrimValue("name", " eD").As("trimmed_name_values"))),
			want:     map[string]interface{}{"trimmed_name_values": "John Do"},
		},
		{
			name:     "LTrim",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(LTrim("name").As("ltrimmed_name"))),
			want:     map[string]interface{}{"ltrimmed_name": "John Doe  "},
		},
		{
			name:     "LTrimValue",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(LTrimValue("name", " J").As("ltrimmed_name_values"))),
			want:     map[string]interface{}{"ltrimmed_name_values": "ohn Doe  "},
		},
		{
			name:     "RTrim",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(RTrim("name").As("rtrimmed_name"))),
			want:     map[string]interface{}{"rtrimmed_name": "  John Doe"},
		},
		{
			name:     "RTrimValue",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(RTrimValue("name", " eD").As("rtrimmed_name_values"))),
			want:     map[string]interface{}{"rtrimmed_name_values": "  John Do"},
		},
		{
			name:     "Split",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Split("csv", ",").As("split_string"))),
			want:     map[string]interface{}{"split_string": []interface{}{"a", "b", "c"}},
		},
		{
			name:     "StringRepeat",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(StringRepeat(ConstantOf("a"), 3).As("repeated"))),
			want:     map[string]interface{}{"repeated": "aaa"},
		},
		{
			name:     "StringReplaceOne",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(StringReplaceOne(ConstantOf("aba"), "a", "c").As("replaced"))),
			want:     map[string]interface{}{"replaced": "cba"},
		},
		{
			name:     "StringReplaceAll",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(StringReplaceAll(ConstantOf("aba"), "a", "c").As("replaced"))),
			want:     map[string]interface{}{"replaced": "cbc"},
		},
		{
			name:     "StringIndexOf",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(StringIndexOf("description", "Firestore").As("index"))),
			want:     map[string]interface{}{"index": int64(10)},
		},
		{
			name:     "LTrim",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(LTrim(ConstantOf("  abc  ")).As("ltrim"))),
			want:     map[string]interface{}{"ltrim": "abc  "},
		},
		{
			name:     "RTrim",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(RTrim(ConstantOf("  abc  ")).As("rtrim"))),
			want:     map[string]interface{}{"rtrim": "  abc"},
		},
		{
			name:     "RegexFind",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(RegexFind("email", "[a-z]+").As("find"))),
			want:     map[string]interface{}{"find": "john"},
		},
		{
			name:     "RegexFindAll",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(RegexFindAll("zipCode", "[0-9]").As("findall"))),
			want:     map[string]interface{}{"findall": []interface{}{"1", "2", "3", "4", "5"}},
		},
		// String filter conditions
		{
			name:     "Like",
			pipeline: client.Pipeline().Collection(coll.ID).Where(Like("name", "%John%")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "StartsWith",
			pipeline: client.Pipeline().Collection(coll.ID).Where(StartsWith("name", "  John")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "EndsWith",
			pipeline: client.Pipeline().Collection(coll.ID).Where(EndsWith("name", "Doe  ")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "RegexContains",
			pipeline: client.Pipeline().Collection(coll.ID).Where(RegexContains("email", "@example\\.com")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "RegexMatch",
			pipeline: client.Pipeline().Collection(coll.ID).Where(RegexMatch("zipCode", "^[0-9]{5}$")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "StringContains",
			pipeline: client.Pipeline().Collection(coll.ID).Where(StringContains("description", "Firestore")),
			want:     []map[string]interface{}{doc1want},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}
			lastStage := test.pipeline.stages[len(test.pipeline.stages)-1]
			lastStageName := lastStage.name()

			if lastStageName == stageNameSelect { // This is a select query
				want, ok := test.want.(map[string]interface{})
				if !ok {
					t.Fatalf("invalid test.want type for select query: %T", test.want)
					return
				}
				if len(docs) != 1 {
					t.Fatalf("expected 1 doc, got %d", len(docs))
					return
				}
				got := docs[0].Data()
				if diff := testutil.Diff(got, want); diff != "" {
					t.Errorf("got: %v, want: %v, diff +want -got: %s", got, want, diff)
				}
			} else if lastStageName == stageNameWhere { // This is a where query (filter condition)
				want, ok := test.want.([]map[string]interface{})
				if !ok {
					t.Fatalf("invalid test.want type for where query: %T", test.want)
					return
				}
				if len(docs) != len(want) {
					t.Fatalf("expected %d doc(s), got %d", len(want), len(docs))
					return
				}
				var gots []map[string]interface{}
				for _, doc := range docs {
					got := doc.Data()
					gots = append(gots, got)
				}
				if diff := testutil.Diff(gots, want); diff != "" {
					t.Errorf("got: %v, want: %v, diff +want -got: %s", gots, want, diff)
				}
			} else {
				t.Fatalf("unknown pipeline stage: %s", lastStageName)
				return
			}
		})
	}

}

func vectorFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"v1": Vector64{1.0, 2.0, 3.0},
		"v2": Vector64{4.0, 5.0, 6.0},
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "VectorLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(VectorLength("v1").As("length"))),
			want:     map[string]interface{}{"length": int64(3)},
		},
		{
			name:     "DotProduct - field and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(DotProduct("v1", FieldOf("v2")).As("dot_product"))),
			want:     map[string]interface{}{"dot_product": float64(1*4 + 2*5 + 3*6)},
		},
		{
			name:     "DotProduct - field and constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(DotProduct("v1", Vector64{4.0, 5.0, 6.0}).As("dot_product"))),
			want:     map[string]interface{}{"dot_product": float64(1*4 + 2*5 + 3*6)},
		},
		{
			name:     "EuclideanDistance - field and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(EuclideanDistance("v1", FieldOf("v2")).As("euclidean"))),
			want:     map[string]interface{}{"euclidean": math.Sqrt(math.Pow(4-1, 2) + math.Pow(5-2, 2) + math.Pow(6-3, 2))},
		},
		{
			name:     "EuclideanDistance - field and constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(EuclideanDistance("v1", Vector64{4.0, 5.0, 6.0}).As("euclidean"))),
			want:     map[string]interface{}{"euclidean": math.Sqrt(math.Pow(4-1, 2) + math.Pow(5-2, 2) + math.Pow(6-3, 2))},
		},
		{
			name:     "CosineDistance - field and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(CosineDistance("v1", FieldOf("v2")).As("cosine"))),
			want:     map[string]interface{}{"cosine": 1 - (32 / (math.Sqrt(14) * math.Sqrt(77)))},
		},
		{
			name:     "CosineDistance - field and constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(CosineDistance("v1", Vector64{4.0, 5.0, 6.0}).As("cosine"))),
			want:     map[string]interface{}{"cosine": 1 - (32 / (math.Sqrt(14) * math.Sqrt(77)))},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
				return
			}
			got := docs[0].Data()
			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}
}

func timestampFuncs(t *testing.T) {
	t.Parallel()
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	h := testHelper{t}
	now := time.Now()
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"timestamp":   now,
		"unixMicros":  now.UnixNano() / 1000,
		"unixMillis":  now.UnixNano() / 1e6,
		"unixSeconds": now.Unix(),
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name: "TimestampAdd day",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampAdd("timestamp", "day", 1).As("timestamp_plus_day"))),
			want: map[string]interface{}{"timestamp_plus_day": now.AddDate(0, 0, 1).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampAdd hour",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampAdd("timestamp", "hour", 1).As("timestamp_plus_hour"))),
			want: map[string]interface{}{"timestamp_plus_hour": now.Add(time.Hour).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampAdd minute",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampAdd("timestamp", "minute", 1).As("timestamp_plus_minute"))),
			want: map[string]interface{}{"timestamp_plus_minute": now.Add(time.Minute).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampAdd second",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampAdd("timestamp", "second", 1).As("timestamp_plus_second"))),
			want: map[string]interface{}{"timestamp_plus_second": now.Add(time.Second).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampSubtract",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampSubtract("timestamp", "hour", 1).As("timestamp_minus_hour"))),
			want: map[string]interface{}{"timestamp_minus_hour": now.Add(-time.Hour).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampToUnixMicros",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(FieldOf("timestamp").TimestampToUnixMicros().As("timestamp_micros"))),
			want: map[string]interface{}{"timestamp_micros": now.UnixNano() / 1000},
		},
		{
			name: "TimestampToUnixMillis",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(FieldOf("timestamp").TimestampToUnixMillis().As("timestamp_millis"))),
			want: map[string]interface{}{"timestamp_millis": now.UnixNano() / 1e6},
		},
		{
			name: "TimestampToUnixSeconds",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(FieldOf("timestamp").TimestampToUnixSeconds().As("timestamp_seconds"))),
			want: map[string]interface{}{"timestamp_seconds": now.Unix()},
		},
		{
			name: "UnixMicrosToTimestamp - constant",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(UnixMicrosToTimestamp(ConstantOf(now.UnixNano() / 1000)).As("timestamp_from_micros"))),
			want: map[string]interface{}{"timestamp_from_micros": now.Truncate(time.Microsecond)},
		},
		{
			name: "UnixMicrosToTimestamp - fieldname",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(UnixMicrosToTimestamp("unixMicros").As("timestamp_from_micros"))),
			want: map[string]interface{}{"timestamp_from_micros": now.Truncate(time.Microsecond)},
		},
		{
			name: "UnixMillisToTimestamp",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(UnixMillisToTimestamp(ConstantOf(now.UnixNano() / 1e6)).As("timestamp_from_millis"))),
			want: map[string]interface{}{"timestamp_from_millis": now.Truncate(time.Millisecond)},
		},
		{
			name: "UnixSecondsToTimestamp",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(UnixSecondsToTimestamp("unixSeconds").As("timestamp_from_seconds"))),
			want: map[string]interface{}{"timestamp_from_seconds": now.Truncate(time.Second)},
		},
		{
			name: "CurrentTimestamp",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(CurrentTimestamp().As("current_timestamp"))),
			want: map[string]interface{}{"current_timestamp": time.Now().Truncate(time.Microsecond)},
		},
		{
			name: "TimestampTruncate day",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampTruncate("timestamp", "day").As("timestamp_trunc_day"))),
			want: map[string]interface{}{"timestamp_trunc_day": time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampTruncate hour",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampTruncate("timestamp", "hour").As("timestamp_trunc_hour"))),
			want: map[string]interface{}{"timestamp_trunc_hour": time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location()).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampTruncate minute",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampTruncate("timestamp", "minute").As("timestamp_trunc_minute"))),
			want: map[string]interface{}{"timestamp_trunc_minute": time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location()).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampTruncate second",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampTruncate("timestamp", "second").As("timestamp_trunc_second"))),
			want: map[string]interface{}{"timestamp_trunc_second": time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, now.Location()).Truncate(time.Microsecond)},
		},
		{
			name: "TimestampTruncateWithTimezone day",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampTruncateWithTimezone("timestamp", "day", "America/New_York").As("timestamp_trunc_day_ny"))),
			want: map[string]interface{}{"timestamp_trunc_day_ny": func() time.Time {
				loc, _ := time.LoadLocation("America/New_York")
				nowInLoc := now.In(loc)
				return time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), 0, 0, 0, 0, loc).Truncate(time.Microsecond)
			}()},
		},
		{
			name: "TimestampExtract year",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampExtract("timestamp", "year").As("year"))),
			want: map[string]interface{}{"year": int64(now.Year())},
		},
		{
			name: "TimestampExtract month",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampExtract("timestamp", "month").As("month"))),
			want: map[string]interface{}{"month": int64(now.Month())},
		},
		{
			name: "TimestampExtractWithTimezone hour",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampExtractWithTimezone("timestamp", "hour", "America/New_York").As("hour_ny"))),
			want: map[string]interface{}{"hour_ny": func() int64 {
				loc, _ := time.LoadLocation("America/New_York")
				return int64(now.In(loc).Hour())
			}()},
		},
		{
			name: "TimestampDiff",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(TimestampDiff(TimestampAdd("timestamp", "day", 1), FieldOf("timestamp"), "day").As("diff"))),
			want: map[string]interface{}{"diff": int64(1)},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()
			margin := 0 * time.Microsecond
			if test.name == "CurrentTimestamp" {
				margin = 5 * time.Second
			}
			if diff := testutil.Diff(got, test.want, cmpopts.EquateApproxTime(margin)); diff != "" {
				t.Errorf("got: %v, want: %v, diff: %s", got, test.want, diff)
			}
		})
	}
}

func arithmeticFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"a": int(1),
		"b": int(2),
		"c": -3,
		"d": 4.5,
		"e": -5.5,
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "Add - left FieldOf, right FieldOf",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Add(FieldOf("a"), FieldOf("b")).As("add"))),
			want:     map[string]interface{}{"add": int64(3)},
		},
		{
			name:     "Add - left FieldOf, right ConstantOf",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Add(FieldOf("a"), ConstantOf(2)).As("add"))),
			want:     map[string]interface{}{"add": int64(3)},
		},
		{
			name:     "Add - left FieldOf, right constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Add(FieldOf("a"), 5).As("add"))),
			want:     map[string]interface{}{"add": int64(6)},
		},
		{
			name:     "Add - left fieldname, right constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Add("a", 5).As("add"))),
			want:     map[string]interface{}{"add": int64(6)},
		},
		{
			name:     "Add - left fieldpath, right constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Add(FieldPath([]string{"a"}), 5).As("add"))),
			want:     map[string]interface{}{"add": int64(6)},
		},
		{
			name:     "Add - left fieldpath, right expression",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Add(FieldPath([]string{"a"}), Add(FieldOf("b"), FieldOf("d"))).As("add"))),
			want:     map[string]interface{}{"add": float64(7.5)},
		},
		{
			name:     "Subtract",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Subtract("a", FieldOf("b")).As("subtract"))),
			want:     map[string]interface{}{"subtract": int64(-1)},
		},
		{
			name:     "Multiply",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Multiply("a", 5).As("multiply"))),
			want:     map[string]interface{}{"multiply": int64(5)},
		},
		{
			name:     "Divide",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Divide("a", FieldOf("d")).As("divide"))),
			want:     map[string]interface{}{"divide": float64(1 / 4.5)},
		},
		{
			name:     "Mod",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Mod("a", FieldOf("b")).As("mod"))),
			want:     map[string]interface{}{"mod": int64(1)},
		},
		{
			name:     "Pow",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Pow("a", FieldOf("b")).As("pow"))),
			want:     map[string]interface{}{"pow": float64(1)},
		},
		{
			name:     "Abs - fieldname",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Abs("c").As("abs"))),
			want:     map[string]interface{}{"abs": int64(3)},
		},
		{
			name:     "Abs - fieldPath",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Abs(FieldPath([]string{"c"})).As("abs"))),
			want:     map[string]interface{}{"abs": int64(3)},
		},
		{
			name:     "Abs - Expr",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Abs(Add(FieldOf("b"), FieldOf("d"))).As("abs"))),
			want:     map[string]interface{}{"abs": float64(6.5)},
		},
		{
			name:     "Ceil",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Ceil("d").As("ceil"))),
			want:     map[string]interface{}{"ceil": float64(5)},
		},
		{
			name:     "Floor",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Floor("d").As("floor"))),
			want:     map[string]interface{}{"floor": float64(4)},
		},
		{
			name:     "Round",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Round("d").As("round"))),
			want:     map[string]interface{}{"round": float64(5)},
		},
		{
			name:     "Sqrt",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Sqrt("d").As("sqrt"))),
			want:     map[string]interface{}{"sqrt": math.Sqrt(4.5)},
		},
		{
			name:     "Log",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Log("d", 2).As("log"))),
			want:     map[string]interface{}{"log": math.Log2(4.5)},
		},
		{
			name:     "Log10",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Log10("d").As("log10"))),
			want:     map[string]interface{}{"log10": math.Log10(4.5)},
		},
		{
			name:     "Ln",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Ln("d").As("ln"))),
			want:     map[string]interface{}{"ln": math.Log(4.5)},
		},
		{
			name:     "Exp",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Exp("d").As("exp"))),
			want:     map[string]interface{}{"exp": math.Exp(4.5)},
		},
		{
			name:     "Trunc",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Trunc("d").As("trunc"))),
			want:     map[string]interface{}{"trunc": float64(4)},
		},
		{
			name:     "TruncToPrecision",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(TruncToPrecision("d", 1).As("trunc_places"))),
			want:     map[string]interface{}{"trunc_places": float64(4.5)},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()
			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}

	t.Run("Rand", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(coll.ID).
			Select(Fields(Rand().As("rand_val"))).
			Limit(1)

		iter := pipeline.Execute(ctx).Results()
		defer iter.Stop()

		res, err := iter.Next()
		if err != nil {
			t.Fatalf("iter.Next() failed: %v", err)
		}

		data := res.Data()
		randVal, ok := data["rand_val"].(float64)
		if !ok {
			t.Fatalf("rand_val is not float64, got %T", data["rand_val"])
		}
		if randVal < 0.0 || randVal >= 1.0 {
			t.Errorf("rand_val %f is out of range [0.0, 1.0)", randVal)
		}
	})
}

func aggregateFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"a": 1,
	})
	docRef2 := coll.NewDoc()
	h.mustCreate(docRef2, map[string]interface{}{
		"a": 2,
	})
	docRef3 := coll.NewDoc()
	h.mustCreate(docRef3, map[string]interface{}{
		"b": 2,
	})
	defer deleteDocuments([]*DocumentRef{docRef1, docRef2, docRef3})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name: "Sum - fieldname arg",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(Sum("a").As("sum_a"))),
			want: map[string]interface{}{"sum_a": int64(3)},
		},
		{
			name: "Sum - fieldpath arg",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(Sum(FieldPath([]string{"a"})).As("sum_a"))),
			want: map[string]interface{}{"sum_a": int64(3)},
		},
		{
			name: "Sum - FieldOf Expr",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(Sum(FieldOf("a")).As("sum_a"))),
			want: map[string]interface{}{"sum_a": int64(3)},
		},
		{
			name: "Sum - FieldOf Path Expr",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(Sum(FieldOf(FieldPath([]string{"a"}))).As("sum_a"))),
			want: map[string]interface{}{"sum_a": int64(3)},
		},
		{
			name: "Avg",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(Average("a").As("avg_a"))),
			want: map[string]interface{}{"avg_a": float64(1.5)},
		},
		{
			name: "Count",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(Count("a").As("count_a"))),
			want: map[string]interface{}{"count_a": int64(2)},
		},
		{
			name: "CountAll",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Aggregate(Accumulators(CountAll().As("count_all"))),
			want: map[string]interface{}{"count_all": int64(3)},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()
			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}
}

func comparisonFuncs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationClient(t)
	now := time.Now()
	coll := client.Collection(collectionIDs.New())
	doc1data := map[string]interface{}{
		"timestamp": now,
		"a":         1,
		"b":         2,
		"c":         -3,
		"d":         4.5,
		"e":         -5.5,
	}
	_, err := coll.Doc("doc1").Create(ctx, doc1data)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	doc2data := map[string]interface{}{
		"timestamp": now,
		"a":         2,
		"b":         2,
		"c":         -3,
		"d":         4.5,
		"e":         -5.5,
	}
	_, err = coll.Doc("doc2").Create(ctx, doc2data)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer deleteDocuments([]*DocumentRef{coll.Doc("doc1"), coll.Doc("doc2")})

	doc1want := map[string]interface{}{"a": int64(1), "b": int64(2), "c": int64(-3), "d": float64(4.5), "e": float64(-5.5), "timestamp": now.Truncate(time.Microsecond)}

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     []map[string]interface{}
	}{
		{
			name: "Equal",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Where(Equal("a", 1)),
			want: []map[string]interface{}{doc1want},
		},
		{
			name: "NotEqual",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Where(NotEqual("a", 2)),
			want: []map[string]interface{}{doc1want},
		},
		{
			name: "LessThan",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Where(LessThan("a", 2)),
			want: []map[string]interface{}{doc1want},
		},
		{
			name: "GreaterThanOrEqual",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Where(GreaterThanOrEqual("a", 1)),
			want: []map[string]interface{}{doc1want, {"a": int64(2), "b": int64(2), "c": int64(-3), "d": float64(4.5), "e": float64(-5.5), "timestamp": now.Truncate(time.Microsecond)}},
		},
		{
			name: "LessThanOrEqual",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Where(LessThanOrEqual("a", 2)),
			want: []map[string]interface{}{doc1want, {"a": int64(2), "b": int64(2), "c": int64(-3), "d": float64(4.5), "e": float64(-5.5), "timestamp": now.Truncate(time.Microsecond)}},
		},
		{
			name: "Cmp",
			pipeline: client.Pipeline().
				Collection(coll.ID).
				Select(Fields(Cmp("a", 1).As("cmp"))),
			want: []map[string]interface{}{{"cmp": int64(0)}, {"cmp": int64(1)}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
			}
			if len(docs) != len(test.want) {
				t.Fatalf("expected %d doc(s), got %d", len(test.want), len(docs))
			}

			var gots []map[string]interface{}
			for _, doc := range docs {
				got := doc.Data()
				if ts, ok := got["timestamp"].(time.Time); ok {
					got["timestamp"] = ts.Truncate(time.Microsecond)
				}
				gots = append(gots, got)
			}

			sort.Slice(gots, func(i, j int) bool {
				v1i, oki := gots[i]["a"]
				v1j, okj := gots[j]["a"]
				if oki && okj {
					return v1i.(int64) < v1j.(int64)
				}
				v2i, oki := gots[i]["cmp"]
				v2j, okj := gots[j]["cmp"]
				if oki && okj {
					return v2i.(int64) < v2j.(int64)
				}
				return false
			})
			sort.Slice(test.want, func(i, j int) bool {
				v1i, oki := test.want[i]["a"]
				v1j, okj := test.want[j]["a"]
				if oki && okj {
					return v1i.(int64) < v1j.(int64)
				}
				v2i, oki := test.want[i]["cmp"]
				v2j, okj := test.want[j]["cmp"]
				if oki && okj {
					return v2i.(int64) < v2j.(int64)
				}
				return false
			})

			if diff := testutil.Diff(gots, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", gots, test.want, diff)
			}
		})
	}
}

func keyFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.Doc("doc1")
	subDocRef1 := docRef1.Collection("sub").Doc("sub1")
	h.mustCreate(docRef1, map[string]interface{}{
		"a": "hello",
		"b": "world",
	})
	h.mustCreate(subDocRef1, map[string]interface{}{
		"c": "sub-hello",
	})
	defer deleteDocuments([]*DocumentRef{subDocRef1, docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "CollectionId",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(GetCollectionID("__name__").As("collectionId"))),
			want:     map[string]interface{}{"collectionId": coll.ID},
		},
		{
			name:     "DocumentId",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(GetDocumentID(docRef1).As("documentId"))),
			want:     map[string]interface{}{"documentId": "doc1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()
			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}
}

func generalFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"a": "hello",
		"b": "world",
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "Length - string literal",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Length(ConstantOf("hello")).As("len"))),
			want:     map[string]interface{}{"len": int64(5)},
		},
		{
			name:     "Length - field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Length("a").As("len"))),
			want:     map[string]interface{}{"len": int64(5)},
		},
		{
			name:     "Length - field path",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Length(FieldPath{"a"}).As("len"))),
			want:     map[string]interface{}{"len": int64(5)},
		},
		{
			name:     "Reverse - string literal",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Reverse(ConstantOf("hello")).As("reverse"))),
			want:     map[string]interface{}{"reverse": "olleh"},
		},
		{
			name:     "Reverse - field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Reverse("a").As("reverse"))),
			want:     map[string]interface{}{"reverse": "olleh"},
		},
		{
			name:     "Reverse - field path",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Reverse(FieldPath{"a"}).As("reverse"))),
			want:     map[string]interface{}{"reverse": "olleh"},
		},
		{
			name:     "Concat - two literals",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Concat(ConstantOf("hello"), ConstantOf("world")).As("concat"))),
			want:     map[string]interface{}{"concat": "helloworld"},
		},
		{
			name:     "Concat - literal and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Concat(ConstantOf("hello"), FieldOf("b")).As("concat"))),
			want:     map[string]interface{}{"concat": "helloworld"},
		},
		{
			name:     "Concat - two fields",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Concat(FieldOf("a"), FieldOf("b")).As("concat"))),
			want:     map[string]interface{}{"concat": "helloworld"},
		},
		{
			name:     "Concat - field and literal",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Concat(FieldOf("a"), ConstantOf("world")).As("concat"))),
			want:     map[string]interface{}{"concat": "helloworld"},
		},
		{
			name:     "CurrentDocument",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(CurrentDocument().As("doc"))),
			want:     map[string]interface{}{"doc": map[string]interface{}{"a": "hello", "b": "world"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}
			if len(docs) != 1 {
				t.Fatalf("expected 1 doc, got %d", len(docs))
			}
			got := docs[0].Data()

			// Remove extra metadata from CurrentDocument test
			if test.name == "CurrentDocument" {
				if doc, ok := got["doc"].(map[string]interface{}); ok {
					delete(doc, "__create_time__")
					delete(doc, "__update_time__")
					delete(doc, "__name__")
				}
			}

			if diff := testutil.Diff(got, test.want); diff != "" {
				t.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
			}
		})
	}
}

func logicalFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.Doc("doc1")
	doc1Data := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": nil,
		"d": true,
		"e": false,
	}
	h.mustCreate(docRef1, doc1Data)

	docRef2 := coll.Doc("doc2")
	doc2Data := map[string]interface{}{
		"a": 1,
		"b": 1,
		"d": true,
		"e": true,
	}
	h.mustCreate(docRef2, doc2Data)
	defer deleteDocuments([]*DocumentRef{docRef1, docRef2})

	doc1Want := map[string]interface{}{
		"a": int64(1),
		"b": int64(2),
		"c": nil,
		"d": true,
		"e": false,
	}
	doc2Want := map[string]interface{}{
		"a": int64(1),
		"b": int64(1),
		"d": true,
		"e": true,
	}

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     interface{}
	}{
		{
			name:     "Conditional - true",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Conditional(Equal(ConstantOf(1), ConstantOf(1)), FieldOf("a"), FieldOf("b")).As("result"))),
			want:     []map[string]interface{}{{"result": int64(1)}, {"result": int64(1)}},
		},
		{
			name:     "Conditional - false",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Conditional(Equal(ConstantOf(1), ConstantOf(0)), FieldOf("a"), FieldOf("b")).As("result"))),
			want:     []map[string]interface{}{{"result": int64(2)}, {"result": int64(1)}},
		},
		{
			name:     "Conditional - field true",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Conditional(Equal(FieldOf("d"), ConstantOf(true)), FieldOf("a"), FieldOf("b")).As("result"))),
			want:     []map[string]interface{}{{"result": int64(1)}, {"result": int64(1)}},
		},
		{
			name:     "Conditional - field false",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Conditional(Equal(FieldOf("e"), ConstantOf(true)), FieldOf("a"), FieldOf("b")).As("result"))),
			want:     []map[string]interface{}{{"result": int64(2)}, {"result": int64(1)}},
		},
		{
			name:     "LogicalMax",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(LogicalMaximum(FieldOf("a"), FieldOf("b")).As("max"))),
			want:     []map[string]interface{}{{"max": int64(2)}, {"max": int64(1)}},
		},
		{
			name:     "LogicalMin",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(LogicalMinimum(FieldOf("a"), FieldOf("b")).As("min"))),
			want:     []map[string]interface{}{{"min": int64(1)}, {"min": int64(1)}},
		},
		{
			name:     "IfError - no error",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(IfError(FieldOf("a"), ConstantOf(100)).As("result"))),
			want:     []map[string]interface{}{{"result": int64(1)}, {"result": int64(1)}},
		},
		{
			name:     "IfError - error",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(Divide("a", 0).IfError(ConstantOf("was error")).As("ifError"))),
			want:     []map[string]interface{}{{"ifError": "was error"}, {"ifError": "was error"}},
		},
		{
			name:     "IfErrorBoolean - no error",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(IfErrorBoolean(Equal(FieldOf("d"), ConstantOf(true)), Equal(ConstantOf(1), ConstantOf(0))).As("result"))),
			want:     []map[string]interface{}{{"result": true}, {"result": true}},
		},
		{
			name:     "IfErrorBoolean - error",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(IfErrorBoolean(Equal(FieldOf("x"), ConstantOf(true)), Equal(ConstantOf(1), ConstantOf(0))).As("result"))),
			want:     []map[string]interface{}{{"result": false}, {"result": false}},
		},
		{
			name:     "IfAbsent - not absent",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(IfAbsent(FieldOf("a"), ConstantOf(100)).As("result"))),
			want:     []map[string]interface{}{{"result": int64(1)}, {"result": int64(1)}},
		},
		{
			name:     "IfAbsent - absent",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(IfAbsent(FieldOf("x"), ConstantOf(100)).As("result"))),
			want:     []map[string]interface{}{{"result": int64(100)}, {"result": int64(100)}},
		},
		{
			name: "And",
			pipeline: client.Pipeline().Collection(coll.ID).Where(
				And(
					Equal(FieldOf("a"), 1),
					Equal(FieldOf("b"), 2),
				),
			),
			want: []map[string]interface{}{doc1Want},
		},
		{
			name: "Or",
			pipeline: client.Pipeline().Collection(coll.ID).Where(
				Or(
					Equal(FieldOf("b"), 2),
					Equal(FieldOf("e"), true),
				),
			),
			want: []map[string]interface{}{doc1Want, doc2Want},
		},
		{
			name: "Not",
			pipeline: client.Pipeline().Collection(coll.ID).Where(
				Not(Equal(FieldOf("b"), 1)),
			),
			want: []map[string]interface{}{doc1Want},
		},
		{
			name: "Xor",
			pipeline: client.Pipeline().Collection(coll.ID).Where(
				Xor(
					Equal(FieldOf("d"), true),
					Equal(FieldOf("e"), true),
				),
			),
			want: []map[string]interface{}{doc1Want},
		},
		{
			name:     "FieldExists",
			pipeline: client.Pipeline().Collection(coll.ID).Where(FieldExists("c")),
			want:     []map[string]interface{}{doc1Want},
		},
		{
			name:     "IsError",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsError(Divide("a", 0))),
			want:     []map[string]interface{}{doc1Want, doc2Want},
		},
		{
			name:     "IsAbsent",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsAbsent("c")),
			want:     []map[string]interface{}{doc2Want},
		},
		{
			name:     "Nor",
			pipeline: client.Pipeline().Collection(coll.ID).Where(Nor(Equal(FieldOf("a"), 1), Equal(FieldOf("b"), 2))),
			want:     []map[string]interface{}(nil),
		},
		{
			name:     "IsType",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsType("a", "int64")),
			want:     []map[string]interface{}{doc1Want, doc2Want},
		},
		{
			name:     "IfNull",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(IfNull(FieldOf("c"), 0).As("result"))),
			want:     []map[string]interface{}{{"result": int64(0)}, {"result": int64(0)}},
		},
		{
			name:     "Switch",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Fields(SwitchOn(Equal(FieldOf("b"), 1), "one", Equal(FieldOf("b"), 2), "two", "other").As("result"))),
			want:     []map[string]interface{}{{"result": "one"}, {"result": "two"}},
		},
		{
			name:     "CountIf",
			pipeline: client.Pipeline().Collection(coll.ID).Aggregate(Accumulators(Equal(FieldOf("b"), 2).CountIf().As("count_b_is_2"))),
			want:     []map[string]interface{}{{"count_b_is_2": int64(1)}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			iter := test.pipeline.Execute(ctx).Results()
			defer iter.Stop()

			docs, err := iter.GetAll()
			if err != nil {
				t.Fatalf("GetAll: %v", err)
				return
			}

			lastStage := test.pipeline.stages[len(test.pipeline.stages)-1]
			lastStageName := lastStage.name()

			if lastStageName == stageNameSelect { // This is a select query
				want, ok := test.want.([]map[string]interface{})
				if !ok {
					t.Fatalf("invalid test.want type for select query: %T", test.want)
					return
				}
				if len(docs) != len(want) {
					t.Fatalf("expected %d doc(s), got %d", len(want), len(docs))
					return
				}
				var gots []map[string]interface{}
				for _, doc := range docs {
					gots = append(gots, doc.Data())
				}
				if diff := testutil.Diff(gots, want, cmpopts.SortSlices(func(a, b map[string]interface{}) bool {
					// A stable sort for the results.
					// Try to sort by "result", "max", "min", "ifError"
					if v1, ok := a["result"]; ok {
						v2 := b["result"]
						switch v1 := v1.(type) {
						case int64:
							return v1 < v2.(int64)
						case bool:
							return !v1 && v2.(bool)
						}
					}
					if v1, ok := a["max"]; ok {
						return v1.(int64) < b["max"].(int64)
					}
					if v1, ok := a["min"]; ok {
						return v1.(int64) < b["min"].(int64)
					}
					if v1, ok := a["ifError"]; ok {
						return v1.(string) < b["ifError"].(string)
					}
					return false
				})); diff != "" {
					t.Errorf("got: %v, want: %v, diff +want -got: %s", gots, want, diff)
				}
			} else if lastStageName == stageNameWhere { // This is a where query (filter condition)
				want, ok := test.want.([]map[string]interface{})
				if !ok {
					t.Fatalf("invalid test.want type for where query: %T", test.want)
					return
				}
				if len(docs) != len(want) {
					t.Fatalf("expected %d doc(s), got %d", len(want), len(docs))
					return
				}
				var gots []map[string]interface{}
				for _, doc := range docs {
					got := doc.Data()
					gots = append(gots, got)
				}
				// Sort slices before comparing for consistent test results
				sort.Slice(gots, func(i, j int) bool {
					if gots[i]["a"].(int64) == gots[j]["a"].(int64) {
						return gots[i]["b"].(int64) < gots[j]["b"].(int64)
					}
					return gots[i]["a"].(int64) < gots[j]["a"].(int64)
				})
				sort.Slice(want, func(i, j int) bool {
					if want[i]["a"].(int64) == want[j]["a"].(int64) {
						return want[i]["b"].(int64) < want[j]["b"].(int64)
					}
					return want[i]["a"].(int64) < want[j]["a"].(int64)
				})
				if diff := testutil.Diff(gots, want); diff != "" {
					t.Errorf("got: %v, want: %v, diff +want -got: %s", gots, want, diff)
				}
			} else if lastStageName == stageNameAggregate { // This is an aggregate query
				want, ok := test.want.([]map[string]interface{})
				if !ok {
					t.Fatalf("invalid test.want type for aggregate query: %T", test.want)
					return
				}
				if len(docs) != len(want) {
					t.Fatalf("expected %d doc(s), got %d", len(want), len(docs))
					return
				}
				var gots []map[string]interface{}
				for _, doc := range docs {
					gots = append(gots, doc.Data())
				}
				if diff := testutil.Diff(gots, want); diff != "" {
					t.Errorf("got: %v, want: %v, diff +want -got: %s", gots, want, diff)
				}
			} else {
				t.Fatalf("unknown pipeline stage: %s", lastStageName)
				return
			}
		})
	}
}

func TestIntegration_PipelineSubqueriesAndVariables(t *testing.T) {
	skipIfNotEnterprise(t)
	ctx := context.Background()
	client := integrationClient(t)
	coll := integrationColl(t)

	// Create test documents
	restaurantsRef := coll.NewDoc()
	_, err := restaurantsRef.Create(ctx, map[string]interface{}{
		"name": "The Burger Joint",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restaurantsRef.Delete(ctx)
	})

	review1Ref := restaurantsRef.Collection("reviews").NewDoc()
	_, err = review1Ref.Create(ctx, map[string]interface{}{
		"reviewer": "Alice",
		"rating":   5,
	})
	if err != nil {
		t.Fatal(err)
	}

	review2Ref := restaurantsRef.Collection("reviews").NewDoc()
	_, err = review2Ref.Create(ctx, map[string]interface{}{
		"reviewer": "Bob",
		"rating":   4,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		review1Ref.Delete(ctx)
		review2Ref.Delete(ctx)
	})

	productsRef := coll.NewDoc()
	_, err = productsRef.Create(ctx, map[string]interface{}{
		"name":  "Widget",
		"price": 100,
		"stock": 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		productsRef.Delete(ctx)
	})

	t.Run("SubcollectionAndScalar", func(t *testing.T) {
		iter := client.Pipeline().Documents([]*DocumentRef{restaurantsRef}).
			AddFields(Selectables(
				Subcollection("reviews").
					Aggregate(Accumulators(Average("rating").As("avg_score"))).
					ToScalarExpression().As("stats"),
			)).Execute(ctx).Results()

		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(res))
		}

		data := res[0].Data()
		stats, ok := data["stats"].(float64)
		if !ok {
			t.Fatalf("expected stats to be float64, got %T", data["stats"])
		}
		if stats != 4.5 {
			t.Errorf("expected stats 4.5, got %v", stats)
		}
	})

	t.Run("SubcollectionAndArray", func(t *testing.T) {
		iter := client.Pipeline().Documents([]*DocumentRef{restaurantsRef}).
			AddFields(Selectables(
				Subcollection("reviews").
					Select(Fields("reviewer", "rating")).
					Sort(Orders(Ascending(FieldOf("reviewer")))).
					ToArrayExpression().As("reviews"),
			)).Execute(ctx).Results()

		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(res))
		}

		data := res[0].Data()
		reviews, ok := data["reviews"].([]interface{})
		if !ok {
			t.Fatalf("expected reviews to be array, got %T", data["reviews"])
		}
		if len(reviews) != 2 {
			t.Fatalf("expected 2 reviews, got %d", len(reviews))
		}
		r1 := reviews[0].(map[string]interface{})
		if r1["reviewer"] != "Alice" || r1["rating"].(int64) != 5 {
			t.Errorf("expected Alice with rating 5, got %v", r1)
		}
	})

	t.Run("DefineAndVariable", func(t *testing.T) {
		iter := client.Pipeline().Documents([]*DocumentRef{productsRef}).
			Define(AliasedExpressions(
				Multiply("price", 0.9).As("discountedPrice"),
				Add("stock", 10).As("newStock"),
			)).
			Where(LessThan(Variable("discountedPrice"), 100)).
			Select(Fields("name", Variable("newStock").As("newStock"))).
			Execute(ctx).Results()

		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(res))
		}

		data := res[0].Data()
		if data["name"] != "Widget" {
			t.Errorf("expected name Widget, got %v", data["name"])
		}
		if data["newStock"].(int64) != 30 {
			t.Errorf("expected newStock 30, got %v", data["newStock"])
		}
	})

	t.Run("CurrentDocument", func(t *testing.T) {
		iter := client.Pipeline().Documents([]*DocumentRef{productsRef}).
			Define(AliasedExpressions(CurrentDocument().As("doc"))).
			Select(Fields(MapGet(Variable("doc"), "name").As("name"))).
			Execute(ctx).Results()

		res, err := iter.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(res))
		}

		data := res[0].Data()
		if data["name"] != "Widget" {
			t.Errorf("expected name Widget, got %v", data["name"])
		}
	})
}
