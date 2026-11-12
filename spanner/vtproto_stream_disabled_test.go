//go:build !spanner_vtproto

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

package spanner

import (
	"context"
	"fmt"
	"slices"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestBorrowRowsIgnoredWithoutBuildTag(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	const sql = "SELECT S FROM Strings"
	var want []string
	rs := &sppb.ResultSet{Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
		{Name: "S", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
	}}}}
	for i := range 10 {
		want = append(want, fmt.Sprintf("string-%d", i))
		rs.Rows = append(rs.Rows, &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue(want[i])}})
	}
	if err := server.TestSpanner.PutStatementResult(sql, &StatementResult{Type: StatementResultResultSet, ResultSet: rs}); err != nil {
		t.Fatal(err)
	}
	iter := client.Single().QueryWithOptions(context.Background(), NewStatement(sql), QueryOptions{ExperimentalBorrowRows: true})
	defer iter.Stop()
	var rows []*Row
	var got []string
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if row.Detach() != row {
			t.Error("Detach copied a row that stays valid")
		}
		var s string
		if err := row.Column(0, &s); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
		got = append(got, s)
	}
	// The rows and strings that were kept without Detach stay valid.
	if !slices.Equal(got, want) {
		t.Errorf("strings = %q, want %q", got, want)
	}
	for i, row := range rows {
		var s string
		if err := row.Column(0, &s); err != nil {
			t.Fatal(err)
		}
		if s != want[i] {
			t.Errorf("row %d = %q, want %q", i, s, want[i])
		}
	}
}
