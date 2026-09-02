// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package bttest

import (
	"context"
	"math"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	btapb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTimestampConversion(t *testing.T) {
	// 1. Test a Timestamp converting to Time.
	var ts1 bigtable.Timestamp = 1583863200000000
	t1 := ts1.Time().UTC()
	want1 := time.Date(2020, time.March, 10, 18, 0, 0, 0, time.UTC)

	if !want1.Equal(t1) {
		t.Errorf("Mismatched time got %v wanted %v", t1, want1)
	}
	// 2. Test a reversed Timestamp converting to Time.
	reverse := math.MaxInt64 - ts1

	got2 := reverse.Time().UTC()
	want2 := time.Date(294196, time.October, 31, 10, 0, 54, 775807000, time.UTC)

	if !want2.Equal(got2) {
		t.Errorf("Mismatched time got %v wanted %v", got2, want2)
	}

	// 3. Test a Time converted to Timestamp then converted back to Time.
	t2 := time.Date(2016, time.October, 3, 14, 7, 7, 0, time.UTC)
	ts2 := bigtable.Timestamp(t2.UnixNano() / 1000)

	got3 := ts2.Time().UTC()
	want3 := time.Date(2016, time.October, 3, 14, 7, 7, 0, time.UTC)
	if !want3.Equal(got3) {
		t.Errorf("Mismatched time got %v wanted %v", got3, want3)
	}
}

// newGranularityServer creates a server holding a single table with the given
// timestamp granularity, and returns the server and the table's name.
func newGranularityServer(t *testing.T, g btapb.Table_TimestampGranularity) (*server, string) {
	t.Helper()
	s := &server{tables: make(map[string]*table)}
	tblInfo, err := s.CreateTable(context.Background(), &btapb.CreateTableRequest{
		Parent:  "cluster",
		TableId: "t",
		Table: &btapb.Table{
			ColumnFamilies: map[string]*btapb.ColumnFamily{"cf": {}},
			Granularity:    g,
		},
	})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	if got := tblInfo.Granularity; got != wantGranularity(g) {
		t.Fatalf("CreateTable granularity: got %v, want %v", got, wantGranularity(g))
	}
	return s, tblInfo.Name
}

func wantGranularity(g btapb.Table_TimestampGranularity) btapb.Table_TimestampGranularity {
	if g == btapb.Table_TIMESTAMP_GRANULARITY_UNSPECIFIED {
		return btapb.Table_MILLIS
	}
	return g
}

func setCell(tableName string, ts int64) *btpb.MutateRowRequest {
	return &btpb.MutateRowRequest{
		TableName: tableName,
		RowKey:    []byte("row"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf",
				ColumnQualifier: []byte("col"),
				TimestampMicros: ts,
				Value:           []byte("v"),
			}},
		}},
	}
}

// readCellTimestamps returns the timestamps of every cell in the table.
func readCellTimestamps(t *testing.T, s *server, tableName string) []int64 {
	t.Helper()
	mock := &MockReadRowsServer{}
	if err := s.ReadRows(&btpb.ReadRowsRequest{TableName: tableName}, mock); err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	var got []int64
	for _, resp := range mock.responses {
		for _, chunk := range resp.Chunks {
			got = append(got, chunk.TimestampMicros)
		}
	}
	return got
}

// TestSetCellTimestampGranularity checks that a microsecond-granularity table
// accepts microsecond timestamps while a millisecond-granularity one rejects
// them.
func TestSetCellTimestampGranularity(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc        string
		granularity btapb.Table_TimestampGranularity
		ts          int64
		wantErr     bool
	}{
		{"millis table, millis timestamp", btapb.Table_MILLIS, 2000, false},
		{"millis table, micros timestamp", btapb.Table_MILLIS, 2001, true},
		{"micros table, millis timestamp", btapb.Table_MICROS, 2000, false},
		{"micros table, micros timestamp", btapb.Table_MICROS, 2001, false},
		{"unspecified table, millis timestamp", btapb.Table_TIMESTAMP_GRANULARITY_UNSPECIFIED, 2000, false},
		{"unspecified table, micros timestamp", btapb.Table_TIMESTAMP_GRANULARITY_UNSPECIFIED, 2001, true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			s, name := newGranularityServer(t, tc.granularity)
			_, err := s.MutateRow(ctx, setCell(name, tc.ts))
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Fatalf("MutateRow(ts=%d) error = %v, wantErr = %v", tc.ts, err, tc.wantErr)
			}
			if tc.wantErr {
				if got := status.Code(err); got != codes.InvalidArgument {
					t.Errorf("MutateRow error code = %v, want %v", got, codes.InvalidArgument)
				}
				return
			}
			if got := readCellTimestamps(t, s, name); len(got) != 1 || got[0] != tc.ts {
				t.Errorf("stored timestamps = %v, want [%d]", got, tc.ts)
			}
		})
	}
}

// TestMaxTimestampGranularity checks the upper bound on timestamps for each
// granularity.
func TestMaxTimestampGranularity(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc        string
		granularity btapb.Table_TimestampGranularity
		wantMax     int64
	}{
		{"millis", btapb.Table_MILLIS, math.MaxInt64 - math.MaxInt64%1000},
		{"micros", btapb.Table_MICROS, math.MaxInt64},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			s, name := newGranularityServer(t, tc.granularity)
			if _, err := s.MutateRow(ctx, setCell(name, tc.wantMax)); err != nil {
				t.Fatalf("MutateRow(ts=%d) unexpected error: %v", tc.wantMax, err)
			}
			// One unit past the max overflows int64 and so must be rejected as
			// a negative timestamp.
			if _, err := s.MutateRow(ctx, setCell(name, tc.wantMax+1)); err == nil {
				t.Fatalf("MutateRow(ts=%d) got no error, want rejection", tc.wantMax+1)
			}
		})
	}
}

// TestServerTimestampIsTruncated checks that server-generated timestamps are
// truncated to the table's granularity rather than rejected.
func TestServerTimestampIsTruncated(t *testing.T) {
	ctx := context.Background()

	// On a millis table, the generated timestamp must be millis-aligned.
	s, name := newGranularityServer(t, btapb.Table_MILLIS)
	before := time.Now().UnixNano() / 1e3
	if _, err := s.MutateRow(ctx, setCell(name, -1)); err != nil {
		t.Fatalf("MutateRow(ServerTime): %v", err)
	}
	after := time.Now().UnixNano() / 1e3
	got := readCellTimestamps(t, s, name)
	if len(got) != 1 {
		t.Fatalf("stored timestamps = %v, want one cell", got)
	}
	if got[0]%1000 != 0 {
		t.Errorf("server timestamp %d is not truncated to millisecond granularity", got[0])
	}
	if got[0] < before-1000 || got[0] > after {
		t.Errorf("server timestamp %d outside [%d, %d]", got[0], before-1000, after)
	}

	// On a micros table, the generated timestamp keeps microsecond precision.
	// It is unit-tested directly since a wall-clock reading happens to be
	// millis-aligned once every thousand times.
	microsTbl := &table{granularity: btapb.Table_MICROS}
	if got, want := microsTbl.timestampUnit(), int64(1); got != want {
		t.Errorf("micros table timestampUnit() = %d, want %d", got, want)
	}
	if ts := microsTbl.newTimestamp(); !microsTbl.validTimestamp(ts) {
		t.Errorf("micros table newTimestamp() = %d, which is not a valid timestamp", ts)
	}
}

// TestDeleteFromColumnTimestampGranularity checks that the timestamp range of
// a DeleteFromColumn mutation is validated against the table's granularity.
func TestDeleteFromColumnTimestampGranularity(t *testing.T) {
	ctx := context.Background()

	deleteRange := func(name string, start, end int64) *btpb.MutateRowRequest {
		return &btpb.MutateRowRequest{
			TableName: name,
			RowKey:    []byte("row"),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_DeleteFromColumn_{DeleteFromColumn: &btpb.Mutation_DeleteFromColumn{
					FamilyName:      "cf",
					ColumnQualifier: []byte("col"),
					TimeRange: &btpb.TimestampRange{
						StartTimestampMicros: start,
						EndTimestampMicros:   end,
					},
				}},
			}},
		}
	}

	// A millis table rejects a micros-granularity delete range.
	s, name := newGranularityServer(t, btapb.Table_MILLIS)
	if _, err := s.MutateRow(ctx, setCell(name, 2000)); err != nil {
		t.Fatalf("MutateRow: %v", err)
	}
	if _, err := s.MutateRow(ctx, deleteRange(name, 1001, 3000)); err == nil {
		t.Error("DeleteFromColumn with micros range on a millis table got no error, want rejection")
	}

	// A micros table accepts it, and the delete takes effect.
	s, name = newGranularityServer(t, btapb.Table_MICROS)
	if _, err := s.MutateRow(ctx, setCell(name, 2001)); err != nil {
		t.Fatalf("MutateRow: %v", err)
	}
	if _, err := s.MutateRow(ctx, deleteRange(name, 2001, 2002)); err != nil {
		t.Fatalf("DeleteFromColumn with micros range on a micros table: %v", err)
	}
	if got := readCellTimestamps(t, s, name); len(got) != 0 {
		t.Errorf("stored timestamps after delete = %v, want none", got)
	}
}

// TestGranularityReportedByAdminAPIs checks that the table's granularity
// survives round-tripping through the admin surface.
func TestGranularityReportedByAdminAPIs(t *testing.T) {
	ctx := context.Background()
	for _, g := range []btapb.Table_TimestampGranularity{btapb.Table_MILLIS, btapb.Table_MICROS} {
		t.Run(g.String(), func(t *testing.T) {
			s, name := newGranularityServer(t, g)

			gt, err := s.GetTable(ctx, &btapb.GetTableRequest{Name: name})
			if err != nil {
				t.Fatalf("GetTable: %v", err)
			}
			if gt.Granularity != g {
				t.Errorf("GetTable granularity = %v, want %v", gt.Granularity, g)
			}

			mt, err := s.ModifyColumnFamilies(ctx, &btapb.ModifyColumnFamiliesRequest{
				Name: name,
				Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
					Id:  "cf2",
					Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{Create: &btapb.ColumnFamily{}},
				}},
			})
			if err != nil {
				t.Fatalf("ModifyColumnFamilies: %v", err)
			}
			if mt.Granularity != g {
				t.Errorf("ModifyColumnFamilies granularity = %v, want %v", mt.Granularity, g)
			}
		})
	}
}

// TestCreateTableRejectsUnknownGranularity checks that an unrecognized
// granularity is rejected at table creation.
func TestCreateTableRejectsUnknownGranularity(t *testing.T) {
	s := &server{tables: make(map[string]*table)}
	_, err := s.CreateTable(context.Background(), &btapb.CreateTableRequest{
		Parent:  "cluster",
		TableId: "t",
		Table:   &btapb.Table{Granularity: btapb.Table_TimestampGranularity(99)},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateTable error = %v, want InvalidArgument", err)
	}
}
