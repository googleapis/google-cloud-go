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
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// Integration-test target selection mirrors the parent bigtable package's
// suite (export_test.go / NewIntegrationEnv): an it.* command-line flag takes
// precedence, falling back to the corresponding GCLOUD_TESTS_* env var. The
// app-profile flag is accelerator-specific (the parent config has no app
// profile field).
var (
	itProject      = flag.String("it.project", "", "project for the accelerator integration test (falls back to GCLOUD_TESTS_GOLANG_PROJECT_ID)")
	itInstance     = flag.String("it.instance", "", "Bigtable instance for the accelerator integration test (falls back to GCLOUD_TESTS_BIGTABLE_INSTANCE)")
	itAppProfile   = flag.String("it.app-profile", "", "Bigtable app profile, optional (falls back to GCLOUD_TESTS_BIGTABLE_APP_PROFILE)")
	itDataEndpoint = flag.String("it.data-endpoint", "", "data-plane endpoint override, optional (falls back to GCLOUD_TESTS_BIGTABLE_DATA_ENDPOINT)")
)

// resolveIT resolves the integration-test target using the flag-then-env
// precedence the parent suite uses, and skips the test when the required
// project/instance are unavailable. channelOpts apply to the accelerator data
// channel; adminOpts apply to the admin client. A non-default universe domain
// (GCLOUD_TESTS_BIGTABLE_UNIVERSE_DOMAIN) is applied to both; a data-endpoint
// override applies only to the data channel.
func resolveIT(t *testing.T) (project, instance, appProfile string, channelOpts, adminOpts []option.ClientOption) {
	t.Helper()
	project = firstNonEmpty(*itProject, os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID"))
	instance = firstNonEmpty(*itInstance, os.Getenv("GCLOUD_TESTS_BIGTABLE_INSTANCE"))
	appProfile = firstNonEmpty(*itAppProfile, os.Getenv("GCLOUD_TESTS_BIGTABLE_APP_PROFILE"))
	if project == "" || instance == "" {
		t.Skip("set -it.project/-it.instance (or GCLOUD_TESTS_GOLANG_PROJECT_ID / GCLOUD_TESTS_BIGTABLE_INSTANCE) to run this prod integration test")
	}
	if ud := os.Getenv("GCLOUD_TESTS_BIGTABLE_UNIVERSE_DOMAIN"); ud != "" {
		channelOpts = append(channelOpts, option.WithUniverseDomain(ud))
		adminOpts = append(adminOpts, option.WithUniverseDomain(ud))
	}
	if de := firstNonEmpty(*itDataEndpoint, os.Getenv("GCLOUD_TESTS_BIGTABLE_DATA_ENDPOINT")); de != "" {
		channelOpts = append(channelOpts, option.WithEndpoint(de))
	}
	return project, instance, appProfile, channelOpts, adminOpts
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// prodColumnFamilies are the families the generator draws from; they are
// pre-created in the throwaway table before any writes. Writing to a family
// absent from the schema is rejected by the backend, so the generator is
// confined to this set.
var prodColumnFamilies = []string{"cf0", "cf1", "cf2"}

// TestReadRows_MutateReadRoundTrip_Prod is a true end-to-end integration test
// against a real Cloud Bigtable instance. It exercises the full accelerator
// daemon on both the write and read paths:
//
//  1. generate a random row (constrained so it survives a real write/read cycle),
//  2. write every cell via MutateRow over the daemon's UDS,
//  3. read the row back via ReadRows over the same UDS and reassemble the chunks,
//  4. assert the read row equals the written one (after canonicalizing to the
//     order Bigtable returns: families by name, columns by qualifier, cells by
//     descending timestamp).
//
// Unlike the fake-backed round-trip test, this proves data actually survives a
// write→read cycle through the vRPC session transport and the real backend,
// which no emulator implements. It is skipped in -short mode and whenever the
// project/instance are unspecified (see resolveIT), so it never runs in the
// default unit suite.
func TestReadRows_MutateReadRoundTrip_Prod(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	project, instance, appProfile, channelOpts, adminOpts := resolveIT(t)

	const (
		seed       = 1
		iterations = 10
	)
	ctx := context.Background()

	// Admin: create a throwaway table with the required families, torn down
	// at the end. Uses standard (non-accelerator) admin RPCs.
	adminClient, err := bigtable.NewAdminClient(ctx, project, instance, adminOpts...)
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	tableName := fmt.Sprintf("accel-rt-%d", time.Now().UnixNano())
	if err := adminClient.CreateTable(ctx, tableName); err != nil {
		t.Fatalf("CreateTable(%s): %v", tableName, err)
	}
	defer func() {
		if err := adminClient.DeleteTable(context.Background(), tableName); err != nil {
			t.Logf("cleanup: DeleteTable(%s): %v", tableName, err)
		}
	}()
	for _, f := range prodColumnFamilies {
		if err := adminClient.CreateColumnFamily(ctx, tableName, f); err != nil {
			t.Fatalf("CreateColumnFamily(%s): %v", f, err)
		}
	}

	// The accelerator carries the full table resource name on every V2
	// request; the daemon itself is scoped to (project, instance, appProfile).
	fullTableName := fmt.Sprintf("projects/%s/instances/%s/tables/%s", project, instance, tableName)

	channel, err := NewChannel(ctx, project, instance, appProfile, "", channelOpts...)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	t.Cleanup(func() { channel.Close() })
	// startRoundTripServer (below) registers server/conn teardown via t.Cleanup.
	conn := startRoundTripServer(t, channel)
	client := btpb.NewBigtableClient(conn)

	r := rand.New(rand.NewSource(seed))
	for i := 0; i < iterations; i++ {
		// A fresh key per iteration keeps writes independent — no cross-run
		// interference and no need to clear state between iterations.
		key := []byte(fmt.Sprintf("row-%d-%d", seed, i))
		want := genProdRow(r, key)

		// Write. The first iterations may race table readiness on a freshly
		// created table, so retry transient failures.
		if err := retryTransient(ctx, func() error {
			_, err := client.MutateRow(ctx, &btpb.MutateRowRequest{
				TableName: fullTableName,
				RowKey:    key,
				Mutations: rowToMutations(want),
			})
			return err
		}); err != nil {
			t.Fatalf("iter %d: MutateRow: %v", i, err)
		}

		// Read back through the daemon's ReadRows path.
		var got *btpb.Row
		if err := retryTransient(ctx, func() error {
			stream, err := client.ReadRows(ctx, &btpb.ReadRowsRequest{
				TableName: fullTableName,
				Rows:      &btpb.RowSet{RowKeys: [][]byte{key}},
			})
			if err != nil {
				return err
			}
			var chunks []*btpb.ReadRowsResponse_CellChunk
			for {
				resp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
				chunks = append(chunks, resp.Chunks...)
			}
			got = reassembleRow(chunks)
			return nil
		}); err != nil {
			t.Fatalf("iter %d: ReadRows: %v", i, err)
		}

		wantC, gotC := canonicalizeRow(want), canonicalizeRow(got)
		if !proto.Equal(wantC, gotC) {
			t.Fatalf("iter %d: mutate/read round-trip mismatch\n wrote: %s\n  read: %s",
				i, prototext.Format(wantC), prototext.Format(gotC))
		}
	}
}

// genProdRow generates a random row constrained so it survives a real
// write→read cycle:
//   - families are a subset of prodColumnFamilies, each used at most once (a
//     real row has one entry per family; the backend merges by name),
//   - qualifiers within a family are distinct,
//   - timestamps within a column are distinct and millisecond-aligned (default
//     table granularity is ms; equal timestamps in a column would collide),
//   - no labels (labels are produced by read-side filters, not writable).
func genProdRow(r *rand.Rand, key []byte) *btpb.Row {
	row := &btpb.Row{Key: key}

	// Pick a non-empty subset of families, preserving prodColumnFamilies order.
	for _, name := range prodColumnFamilies {
		if r.Intn(2) == 0 {
			continue
		}
		fam := &btpb.Family{Name: name}
		usedQual := map[string]bool{}
		for c := 0; c < 1+r.Intn(3); c++ {
			q := uniqueBytes(r, usedQual, 1, 6)
			col := &btpb.Column{Qualifier: q}
			usedTs := map[int64]bool{}
			for k := 0; k < 1+r.Intn(3); k++ {
				ts := uniqueMillisTs(r, usedTs)
				col.Cells = append(col.Cells, &btpb.Cell{
					TimestampMicros: ts,
					Value:           randBytes(r, 1, 8),
				})
			}
			fam.Columns = append(fam.Columns, col)
		}
		row.Families = append(row.Families, fam)
	}
	// Guarantee at least one family so the row is non-empty and round-trips.
	if len(row.Families) == 0 {
		row.Families = append(row.Families, &btpb.Family{
			Name: prodColumnFamilies[0],
			Columns: []*btpb.Column{{
				Qualifier: []byte("q"),
				Cells:     []*btpb.Cell{{TimestampMicros: 1000, Value: []byte("v")}},
			}},
		})
	}
	return row
}

// uniqueBytes returns a byte slice of random length in [min,max] not already
// present in used (by string key), recording it.
func uniqueBytes(r *rand.Rand, used map[string]bool, min, max int) []byte {
	for {
		b := randBytes(r, min, max)
		if !used[string(b)] {
			used[string(b)] = true
			return b
		}
	}
}

// uniqueMillisTs returns a distinct, positive, millisecond-aligned timestamp
// in micros, recording it in used.
func uniqueMillisTs(r *rand.Rand, used map[int64]bool) int64 {
	for {
		ts := (1 + r.Int63n(1_000_000)) * 1000
		if !used[ts] {
			used[ts] = true
			return ts
		}
	}
}

// rowToMutations flattens a Row into SetCell mutations for a MutateRow write.
func rowToMutations(row *btpb.Row) []*btpb.Mutation {
	var muts []*btpb.Mutation
	for _, fam := range row.Families {
		for _, col := range fam.Columns {
			for _, cell := range col.Cells {
				muts = append(muts, &btpb.Mutation{Mutation: &btpb.Mutation_SetCell_{
					SetCell: &btpb.Mutation_SetCell{
						FamilyName:      fam.Name,
						ColumnQualifier: col.Qualifier,
						TimestampMicros: cell.TimestampMicros,
						Value:           cell.Value,
					},
				}})
			}
		}
	}
	return muts
}

// canonicalizeRow returns a copy of row sorted into the order Bigtable returns
// on read: families by name, columns by qualifier, cells by descending
// timestamp. This lets a written row be compared with the read-back row by
// proto.Equal regardless of the order mutations were applied in.
func canonicalizeRow(row *btpb.Row) *btpb.Row {
	if row == nil {
		return nil
	}
	out := proto.Clone(row).(*btpb.Row)
	sort.SliceStable(out.Families, func(i, j int) bool {
		return out.Families[i].Name < out.Families[j].Name
	})
	for _, fam := range out.Families {
		sort.SliceStable(fam.Columns, func(i, j int) bool {
			return bytes.Compare(fam.Columns[i].Qualifier, fam.Columns[j].Qualifier) < 0
		})
		for _, col := range fam.Columns {
			sort.SliceStable(col.Cells, func(i, j int) bool {
				return col.Cells[i].TimestampMicros > col.Cells[j].TimestampMicros
			})
		}
	}
	return out
}

// retryTransient retries fn on error codes that are expected while a freshly
// created table becomes readable/writable, with a bounded backoff.
func retryTransient(ctx context.Context, fn func() error) error {
	const attempts = 8
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		switch status.Code(err) {
		case codes.Unavailable, codes.FailedPrecondition, codes.NotFound, codes.DeadlineExceeded:
			timer := time.NewTimer(time.Duration(1<<uint(i)) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		default:
			return err
		}
	}
	return err
}

// startRoundTripServer boots an Server backed by channel on a temp
// UDS and returns a connected real gRPC client conn. Teardown is registered
// via t.Cleanup.
func startRoundTripServer(t *testing.T, channel *Channel) *grpc.ClientConn {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "accelerator-roundtrip-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	udsPath := filepath.Join(tmpDir, "bt_proxy.sock")

	server := NewServer(udsPath, channel, WithStdinReader(nil)) // disable the stdin-EOF watchdog for the test
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start: %v", err)
	}
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("unix://"+udsPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// reassembleRow inverts ReadRowResponseAdapter.Adapt: it rebuilds a Row from
// the CellChunk stream using the adapter's boundary-marker contract — RowKey
// on the first chunk, a FamilyName wrapper at each family transition, a
// Qualifier wrapper at each column transition. This mirrors that specific
// adapter, not general Bigtable chunk-merging semantics (it does not, for
// example, coalesce adjacent same-name families).
func reassembleRow(chunks []*btpb.ReadRowsResponse_CellChunk) *btpb.Row {
	if len(chunks) == 0 {
		return nil
	}
	row := &btpb.Row{Key: chunks[0].RowKey}
	var curFam *btpb.Family
	var curCol *btpb.Column
	for _, cc := range chunks {
		if cc.FamilyName != nil {
			curFam = &btpb.Family{Name: cc.FamilyName.Value}
			row.Families = append(row.Families, curFam)
			curCol = nil // a new family always starts a new column
		}
		if cc.Qualifier != nil {
			curCol = &btpb.Column{Qualifier: cc.Qualifier.Value}
			curFam.Columns = append(curFam.Columns, curCol)
		}
		curCol.Cells = append(curCol.Cells, &btpb.Cell{
			TimestampMicros: cc.TimestampMicros,
			Value:           cc.Value,
			Labels:          cc.Labels,
		})
	}
	return row
}

// randBytes returns a byte slice of random length in [min, max].
func randBytes(r *rand.Rand, min, max int) []byte {
	b := make([]byte, min+r.Intn(max-min+1))
	r.Read(b)
	return b
}
