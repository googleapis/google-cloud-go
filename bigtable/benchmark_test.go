package bigtable

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

/*
To run benchmark tests,
go test -v -timeout 45m  -run=^$ -bench 'BenchmarkReadRows'

View generated CPU profile in browser:
go tool pprof -http=:5051 cpu-profile-WithMetrics.prof
go tool pprof -http=:5052 cpu-profile-WithoutMetrics.prof

View generated CPU profile diff in browser:
go tool pprof -http=:5053 -diff_base=cpu-profile-WithoutMetrics.prof cpu-profile-WithMetrics.prof
*/
const (
	numRows          = 10
	numGoRoutines    = 10
	project          = "cndb-sdk-golang-general"
	columnFamilyName = "cf1"
	columnName       = "greeting"
	instance         = "with-profiling"
	cpuProfilePrefix = "cpu-profile-"
)

// customErrorHandler is a custom implementation of an error handler.
type customErrorHandler struct{}

// Handle is called for internal OpenTelemetry errors.
func (h *customErrorHandler) Handle(err error) {
	fmt.Printf("OpenTelemetry internal error: %v", err)
}

func sliceContains(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func createBenchmarkTable(ctx context.Context, adminClient *AdminClient, tableName string) error {
	tables, err := adminClient.Tables(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch table list: %w", err)
	}

	if !sliceContains(tables, tableName) {
		if err := adminClient.CreateTable(ctx, tableName); err != nil {
			return fmt.Errorf("could not create table %s: %w", tableName, err)
		}
	}

	tblInfo, err := adminClient.TableInfo(ctx, tableName)
	if err != nil {
		return fmt.Errorf("could not read info for table %s: %w", tableName, err)
	}

	if !sliceContains(tblInfo.Families, columnFamilyName) {
		if err := adminClient.CreateColumnFamily(ctx, tableName, columnFamilyName); err != nil {
			return fmt.Errorf("could not create column family %s: %w", columnFamilyName, err)
		}
	}
	return nil
}

// Write numRows to table
func applyBulk(client *Client, tableName, rowKeyPrefix string) error {
	tbl := client.Open(tableName)
	muts := make([]*Mutation, numRows)
	rowKeys := make([]string, numRows)
	for i := 0; i < numRows; i++ {
		muts[i] = NewMutation()
		muts[i].Set(columnFamilyName, columnName, Now(), []byte("Hello"))

		rowKey := generateDeterministicRowKey(rowKeyPrefix, i)
		rowKeys[i] = rowKey
	}

	rowErrs, err := tbl.ApplyBulk(context.Background(), rowKeys, muts)
	if err != nil {
		return fmt.Errorf("could not apply bulk row mutation: %w", err)
	}
	if rowErrs != nil {
		for _, rowErr := range rowErrs {
			fmt.Printf("Error writing row: %v\n", rowErr)
		}
		return fmt.Errorf("could not write some rows")
	}
	return nil
}

func generateDeterministicRowKey(rowKeyPrefix string, index int) string {
	return fmt.Sprintf("%s-%010d", rowKeyPrefix, index)
}

func readRandomRowsIndividually(client *Client, tableName, rowKeyPrefix string) {
	tbl := client.Open(tableName)
	for i := 0; i < numRows; i++ {
		// read a random row
		rowKey := generateDeterministicRowKey(rowKeyPrefix, i)
		_, err := tbl.ReadRow(context.Background(), rowKey, RowFilter(ColumnFilter(columnName)))
		if err != nil {
			fmt.Printf("Could not read row with key %s: %v\n", rowKey, err)
		}
	}
}

func runWorkload(client *Client, tableName, rowKeyPrefix string) {
	var wg sync.WaitGroup
	for i := 0; i < numGoRoutines; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			readRandomRowsIndividually(client, tableName, rowKeyPrefix)
		}(&wg)
	}
	wg.Wait()
}

func setup(b *testing.B, enableMetrics bool) (*Client, string, string, func()) {
	b.Helper()
	ctx := context.Background()

	adminClient, err := NewAdminClient(ctx, project, instance)
	if err != nil {
		b.Fatalf("Failed to create admin client: %v", err)
	}

	config := ClientConfig{MetricsProvider: NoopMetricsProvider{}}
	if enableMetrics {
		config = ClientConfig{}
	}
	client, err := NewClientWithConfig(ctx, project, instance, config)
	if err != nil {
		b.Fatalf("Failed to create data client: %v", err)
	}

	tableName := "profile-" + uuid.NewString()
	if err := createBenchmarkTable(ctx, adminClient, tableName); err != nil {
		b.Fatalf("Failed to create benchmark table: %v", err)
	}

	rowKeyPrefix := "row-" + uuid.New().String()
	if err := applyBulk(client, tableName, rowKeyPrefix); err != nil {
		b.Fatalf("Failed to apply bulk data: %v", err)
	}

	cleanup := func() {
		client.Close()
	}
	return client, tableName, rowKeyPrefix, cleanup
}

func BenchmarkReadRows(b *testing.B) {
	otel.SetErrorHandler(&customErrorHandler{})

	cases := []struct {
		name          string
		enableMetrics bool
	}{
		{name: "WithMetrics", enableMetrics: true},
		{name: "WithoutMetrics", enableMetrics: false},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			client, tableName, rowKeyPrefix, cleanup := setup(b, bc.enableMetrics)
			defer cleanup()

			// Start profiling
			profileFile := cpuProfilePrefix + bc.name + ".prof"
			f, err := os.Create(profileFile)
			if err != nil {
				b.Fatalf("Could not create CPU profile file: %v", err)
			}
			defer f.Close()

			if err := pprof.StartCPUProfile(f); err != nil {
				b.Fatalf("Could not start CPU profile: %v", err)
			}
			defer pprof.StopCPUProfile()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				runWorkload(client, tableName, rowKeyPrefix)
			}
		})
	}
}
