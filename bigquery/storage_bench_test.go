// Copyright 2023 Google LLC
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

package bigquery

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/api/iterator"
)

func BenchmarkIntegration_StorageReadQuery(b *testing.B) {
	if storageOptimizedClient == nil {
		b.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	benchCases := []struct {
		name   string
		filter string
	}{
		{name: "usa_1910_current_full", filter: ""},
		{name: "usa_1910_current_state_eq_fl", filter: "where state = \"FL\""},
		{name: "usa_1910_current_state_eq_ca", filter: "where state = \"CA\""},
		{name: "usa_1910_current_full_ordered", filter: "order by name"},
	}

	type S struct {
		Name   string
		Number int
		State  string
		Nested struct {
			Name string
			N    int
		}
	}

	for _, bc := range benchCases {
		sql := fmt.Sprintf(`SELECT name, number, state, STRUCT(name as name, number as n) as nested FROM %s %s`, table, bc.filter)
		for _, maxStreamCount := range []int{0, 1} {
			storageOptimizedClient.rc.settings.maxStreamCount = maxStreamCount
			b.Run(fmt.Sprintf("storage_api_%d_max_streams_%s", maxStreamCount, bc.name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					q := storageOptimizedClient.Query(sql)
					q.forceStorageAPI = true
					it, err := q.Read(ctx)
					if err != nil {
						b.Fatal(err)
					}
					if !it.IsAccelerated() {
						b.Fatal("expected query execution to be accelerated")
					}
					for {
						var s S
						err := it.Next(&s)
						if err == iterator.Done {
							break
						}
						if err != nil {
							b.Fatalf("failed to fetch via storage API: %v", err)
						}
					}
					b.ReportMetric(float64(it.TotalRows), "rows")
					bqSession := it.arrowIterator.(*storageArrowIterator).session.bqSession
					b.ReportMetric(float64(len(bqSession.Streams)), "parallel_streams")
					b.ReportMetric(float64(maxStreamCount), "max_streams")
				}
			})
		}
		b.Run(fmt.Sprintf("rest_api_%s", bc.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				q := client.Query(sql)
				it, err := q.Read(ctx)
				if err != nil {
					b.Fatal(err)
				}
				for {
					var s S
					err := it.Next(&s)
					if err == iterator.Done {
						break
					}
					if err != nil {
						b.Fatalf("failed to fetch via query API: %v", err)
					}
				}
				b.ReportMetric(float64(it.TotalRows), "rows")
				b.ReportMetric(1, "parallel_streams")
			}
		})
	}
}
