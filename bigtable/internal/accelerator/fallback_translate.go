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

package accelerator

import (
	"sort"
	"strings"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// rowToReadRowsResponse converts a bigtable.Row (Go struct returned by the
// classic client) into a ReadRowsResponse for the proxy stream.
//
// The encoding mirrors adapters/read_row.go: one CellChunk per cell, with
// RowKey on the first chunk, FamilyName on family transitions, Qualifier on
// column transitions, and CommitRow on the last chunk.
//
// An empty or nil Row (missing key) returns an empty ReadRowsResponse with no
// chunks. The proxy stream's RecvMsg returns this as a nil response followed
// by EOF, matching the standard Bigtable "key not found" contract.
func rowToReadRowsResponse(row bigtable.Row, rowKey []byte) *v2pb.ReadRowsResponse {
	if len(row) == 0 {
		return &v2pb.ReadRowsResponse{}
	}

	// Sort families for deterministic output; bigtable.Row is a map.
	families := make([]string, 0, len(row))
	for f := range row {
		families = append(families, f)
	}
	sort.Strings(families)

	var chunks []*v2pb.ReadRowsResponse_CellChunk
	firstChunk := true
	prevFamily := ""
	prevQualifier := ""

	for _, fam := range families {
		for _, item := range row[fam] {
			// ReadItem.Column is "family:qualifier"; split on the first colon.
			_, qual, _ := strings.Cut(item.Column, ":")

			chunk := &v2pb.ReadRowsResponse_CellChunk{
				TimestampMicros: int64(item.Timestamp),
				Value:           item.Value,
				Labels:          item.Labels,
			}
			if firstChunk {
				chunk.RowKey = rowKey
				firstChunk = false
			}
			if fam != prevFamily {
				chunk.FamilyName = &wrapperspb.StringValue{Value: fam}
				prevFamily = fam
				// Qualifier always resets on a family transition.
				chunk.Qualifier = &wrapperspb.BytesValue{Value: []byte(qual)}
				prevQualifier = qual
			} else if qual != prevQualifier {
				chunk.Qualifier = &wrapperspb.BytesValue{Value: []byte(qual)}
				prevQualifier = qual
			}
			chunks = append(chunks, chunk)
		}
	}

	if len(chunks) > 0 {
		chunks[len(chunks)-1].RowStatus = &v2pb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true}
	}
	return &v2pb.ReadRowsResponse{Chunks: chunks}
}
