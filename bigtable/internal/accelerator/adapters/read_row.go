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

package adapters

import (
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ReadRowRequestAdapter adapts V2 ReadRowsRequest to SessionReadRowRequest.
type ReadRowRequestAdapter struct{}

// Adapt converts a V2 ReadRowsRequest into a SessionReadRowRequest for a point
// read, carrying over the filter and extracting the single row key from either
// RowKeys[0] or the closed start key of RowRanges[0]. It returns nil for a nil
// input.
func (a *ReadRowRequestAdapter) Adapt(from *v2pb.ReadRowsRequest) (*v2pb.SessionReadRowRequest, error) {
	if from == nil {
		return nil, nil
	}
	req := &v2pb.SessionReadRowRequest{Filter: from.Filter}
	if from.Rows == nil {
		return req, nil
	}
	switch {
	case len(from.Rows.RowKeys) > 0:
		req.Key = from.Rows.RowKeys[0]
	case len(from.Rows.RowRanges) > 0:
		if r, ok := from.Rows.RowRanges[0].StartKey.(*v2pb.RowRange_StartKeyClosed); ok {
			req.Key = r.StartKeyClosed
		}
	}
	return req, nil
}

// ExtractResource returns the resource the request targets, tagged with its
// kind. A ReadRowsRequest names exactly one of a table, an authorized view, or
// a materialized view; the more-specific fields are checked first so the
// correct resource is surfaced regardless of which the caller populated.
func (a *ReadRowRequestAdapter) ExtractResource(from *v2pb.ReadRowsRequest) (Resource, error) {
	if from == nil {
		return Resource{}, status.Errorf(codes.InvalidArgument, "request is nil")
	}
	switch {
	case from.MaterializedViewName != "":
		return Resource{Kind: ResourceMaterializedView, Name: from.MaterializedViewName}, nil
	case from.AuthorizedViewName != "":
		return Resource{Kind: ResourceAuthorizedView, Name: from.AuthorizedViewName}, nil
	case from.TableName != "":
		return Resource{Kind: ResourceTable, Name: from.TableName}, nil
	default:
		return Resource{}, status.Errorf(codes.InvalidArgument, "ReadRowsRequest names no table, authorized view, or materialized view")
	}
}

// ReadRowResponseAdapter adapts SessionReadRowResponse to ReadRowsResponse,
// flattening Row.Families → Columns → Cells into a sequence of CellChunks
// with the on-wire boundary markers (RowKey on the first chunk of the row,
// FamilyName at each family transition, Qualifier at each column transition,
// CommitRow on the last chunk).
type ReadRowResponseAdapter struct{}

// Adapt converts a SessionReadRowResponse into a V2 ReadRowsResponse, flattening
// the row's families, columns, and cells into CellChunks with the on-wire
// boundary markers. It returns nil when the input or its row is nil, or when the
// row contains no cells.
func (a *ReadRowResponseAdapter) Adapt(from *v2pb.SessionReadRowResponse) (*v2pb.ReadRowsResponse, error) {
	if from == nil || from.Row == nil {
		return nil, nil
	}
	row := from.Row

	var chunks []*v2pb.ReadRowsResponse_CellChunk
	first := true
	for _, fam := range row.Families {
		familyEmitted := false
		for _, col := range fam.Columns {
			columnEmitted := false
			for _, cell := range col.Cells {
				cc := &v2pb.ReadRowsResponse_CellChunk{
					TimestampMicros: cell.TimestampMicros,
					Value:           cell.Value,
					Labels:          cell.Labels,
				}
				if first {
					cc.RowKey = row.Key
					first = false
				}
				if !familyEmitted {
					cc.FamilyName = &wrapperspb.StringValue{Value: fam.Name}
					familyEmitted = true
				}
				if !columnEmitted {
					cc.Qualifier = &wrapperspb.BytesValue{Value: col.Qualifier}
					columnEmitted = true
				}
				chunks = append(chunks, cc)
			}
		}
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	chunks[len(chunks)-1].RowStatus = &v2pb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true}
	return &v2pb.ReadRowsResponse{Chunks: chunks}, nil
}
