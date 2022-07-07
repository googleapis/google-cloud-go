// Copyright 2017 Google LLC
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

//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// profileTag is a simple annotation for benchmark runs.
type profileTag struct {
	Key   string `json:"key,omitempty" bigquery:"key"`
	Value string `json:"value,omitempty" bigquery:"value"`
}

type tags []*profileTag

func (ts *tags) String() string {
	var s strings.Builder
	fp := len(*ts)
	for i, t := range *ts {
		s.WriteString(fmt.Sprintf("%s:%s", t.Key, t.Value))
		if i < fp-1 {
			s.WriteString(",")
		}
	}
	return s.String()
}

func (ts *tags) Set(value string) error {
	if value == "" {
		return nil
	}
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 2 {
		// both a key and value
		*ts = append(*ts, &profileTag{Key: parts[0], Value: parts[1]})
	} else {
		*ts = append(*ts, &profileTag{Key: value})
	}
	return nil
}

// AsSlice is used to simplify schema inference.
func (ts *tags) AsSlice() []*profileTag {
	var out []*profileTag
	for _, v := range *ts {
		out = append(out, v)
	}
	return out
}

// profiledQuery provides metadata about query invocations and performance.
type profiledQuery struct {
	// Used to describe a set of related queries.
	GroupName string `json:"groupname" bigquery:"groupname"`
	// User to describe a single query configuration.
	Name string `json:"name" bigquery:"name"`
	// Tags allow an arbitrary list of KV pairs for denoting specifics of a profile.
	Tags []*profileTag `json:"tags" bigquery:"tags"`
	// Persisted query configuration.
	Query *bigquery.Query `json:"-" bigquery:"-"`
	// Just the query string.
	SQL string
	// Timing details from multiple invocations.
	Runs []*timingInfo `json:"runs" bigquery:"runs"`
	// When this data was logged.
	EventTime time.Time `json:"event_time" bigquery:"event_time"`
}

// timingInfo provides measurements for a single invocation of a query.
type timingInfo struct {
	// If the query failed in error, this retains a copy of the error string
	ErrorString string `json:"errorstring,omitempty" bigquery:"errorstring"`
	// Start time from the client perspective, e.q. calling Read() to insert and wait for an iterator
	StartTime time.Time `json:"start_time,omitempty" bigquery:"start_time"`
	// Measured when the Read() call returns.
	QueryEndTime time.Time `json:"query_end_time,omitempty" bigquery:"query_end_time"`
	// Measured when consumer receives the first row via the iterator.
	FirstRowReturnedTime time.Time `json:"first_row_returned_time,omitempty" bigquery:"first_row_returned_time"`
	// Measured when consumer receives iterator.Done
	AllRowsReturnedTime time.Time `json:"all_rows_returned_time,omitempty" bigquery:"all_rows_returned_time"`
	// Number of rows fetched through the iterator.
	TotalRows int64 `json:"total_rows,omitempty" bigquery:"total_rows"`
}

// Summary provides a human-readable string that summarizes the significant timing details.
func (t *timingInfo) Summary() string {
	noVal := "NODATA"
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "QUERYTIME ")
	if !t.QueryEndTime.IsZero() {
		fmt.Fprintf(&buf, "%v", t.QueryEndTime.Sub(t.StartTime))
	} else {
		fmt.Fprintf(&buf, noVal)
	}

	fmt.Fprintf(&buf, " FIRSTROW ")
	if !t.FirstRowReturnedTime.IsZero() {
		fmt.Fprintf(&buf, "%v (+%v)", t.FirstRowReturnedTime.Sub(t.StartTime), t.FirstRowReturnedTime.Sub(t.QueryEndTime))
	} else {
		fmt.Fprintf(&buf, noVal)
	}

	fmt.Fprintf(&buf, " ALLROWS ")
	if !t.AllRowsReturnedTime.IsZero() {
		fmt.Fprintf(&buf, "%v (+%v)", t.AllRowsReturnedTime.Sub(t.StartTime), t.AllRowsReturnedTime.Sub(t.FirstRowReturnedTime))
	} else {
		fmt.Fprintf(&buf, noVal)
	}
	if t.TotalRows > 0 {
		fmt.Fprintf(&buf, " ROWS %d", t.TotalRows)
	}
	if t.ErrorString != "" {
		fmt.Fprintf(&buf, " ERRORED %s ", t.ErrorString)
	}
	return buf.String()
}

// measureSelectQuery invokes a query given a config and returns timing information.
//
// This instrumentation is meant for the common query case.
func measureSelectQuery(ctx context.Context, q *bigquery.Query) *timingInfo {
	timing := &timingInfo{
		StartTime: time.Now(),
	}
	it, err := q.Read(ctx)
	timing.QueryEndTime = time.Now()
	if err != nil {
		timing.ErrorString = err.Error()
		return timing
	}
	var row []bigquery.Value
	var rowCount int64
	for {
		err := it.Next(&row)
		if rowCount == 0 {
			timing.FirstRowReturnedTime = time.Now()
		}
		if err == iterator.Done {
			timing.AllRowsReturnedTime = time.Now()
			timing.TotalRows = rowCount
			break
		}
		if err != nil {
			timing.ErrorString = err.Error()
			return timing
		}
		rowCount++
	}
	return timing
}

// runBenchmarks processes the input file and instruments the queries.
// It currently instruments queries serially to reduce variance due to concurrent execution on either the backend or in this client.
func runBenchmarks(ctx context.Context, client *bigquery.Client, filename string, tags *tags, reruns int) (profiles []*profiledQuery, err error) {

	queriesJSON, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read queries files: %v", err)
	}

	var benchmarkInput map[string]map[string]string
	if err := json.Unmarshal(queriesJSON, &benchmarkInput); err != nil {
		return nil, fmt.Errorf("failed to unmarshall queries data: %v", err)
	}

	convertedTags := tags.AsSlice()

	for groupName, m := range benchmarkInput {
		for id, sql := range m {
			prof := &profiledQuery{
				GroupName: groupName,
				Name:      id,
				SQL:       sql,
				Tags:      convertedTags,
				EventTime: time.Now(),
			}
			fmt.Printf("Measuring %s : %s", groupName, id)
			query := client.Query(sql)
			prof.Query = query

			for i := 0; i < reruns; i++ {
				fmt.Printf(".")
				prof.Runs = append(prof.Runs, measureSelectQuery(ctx, query))
			}
			fmt.Println()
			profiles = append(profiles, prof)
		}
	}
	fmt.Println()
	return profiles, nil
}

// printResults prints information about collected query profiles.
func printResults(queries []*profiledQuery) {
	for i, prof := range queries {
		fmt.Printf("%d: (%s:%s)\n", i, prof.GroupName, prof.Name)
		fmt.Printf("SQL: %s\n", prof.Query.Q)
		fmt.Printf("MEASUREMENTS\n")
		for j, timing := range prof.Runs {
			fmt.Printf("\t\t(%d) %s\n", j, timing.Summary())
		}
		fmt.Println()
	}
}

// prepareTable ensures a table exists, and optionally creates it if directed
func prepareTable(ctx context.Context, client *bigquery.Client, table string, create bool) (*bigquery.Table, error) {
	// Ensure table exists before streaming results, and possibly create it if directed.
	parts := strings.Split(table, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("Expected table in p.d.t format, got: %s", table)
	}
	tRef := client.DatasetInProject(parts[0], parts[1]).Table(parts[2])
	// check with backend
	_, err := tRef.Metadata(ctx)
	if err != nil {
		if create {
			schema, err := bigquery.InferSchema(profiledQuery{})
			if err != nil {
				return nil, fmt.Errorf("could not infer schema while creating table: %v", err)
			}
			createMeta := &bigquery.TableMetadata{
				Schema: schema.Relax(),
				TimePartitioning: &bigquery.TimePartitioning{
					Type:  bigquery.DayPartitioningType,
					Field: "event_time",
				},
				Clustering: &bigquery.Clustering{
					Fields: []string{"groupname", "name"},
				},
			}
			if err2 := tRef.Create(ctx, createMeta); err2 != nil {
				return nil, fmt.Errorf("could not create table: %v", err2)
			}
			return tRef, nil
		}
		return nil, fmt.Errorf("error while validating table existence: %v", err)
	}
	return tRef, nil
}

// reportResults streams results into the designated table.
func reportResults(ctx context.Context, client *bigquery.Client, table *bigquery.Table, results []*profiledQuery) error {
	inserter := table.Inserter()

	// Set a timeout on our context to bound retries
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := inserter.Put(ctx, results); err != nil {
		return fmt.Errorf("reportResults: %v", err)
	}
	return nil
}

func main() {
	var reruns = flag.Int("reruns", 3, "number of reruns to issue for each query")
	var queryfile = flag.String("queryfile", "benchmarked-queries.json", "path to file contain queries to be benchmarked.")
	var projectID = flag.String("projectid", "", "project ID to use for running benchmarks.  Uses GOOGLE_CLOUD_PROJECT env if not set.")
	var reportTable = flag.String("table", "", "table to stream results into, specified in project.dataset.table format")
	var createTable = flag.Bool("create_table", false, "create result table if it does not exist")

	var tags tags
	flag.Var(&tags, "tag", "an optional key and value seperated by colon (:) character")
	flag.Parse()

	// Validate flags.
	if *reruns <= 0 {
		log.Fatalf("--reruns should be a positive value")
	}
	projID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if *projectID != "" {
		projID = *projectID
	}
	if projID == "" {
		log.Fatalf("must provide --projectid or set GOOGLE_CLOUD_PROJECT environment variable")
	}

	// Setup context and client based on ADC.
	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, projID)
	if err != nil {
		log.Fatalf("bigquery.NewClient: %v", err)
	}
	defer client.Close()

	// If we're going to stream results, let's make sure we can do that before running all the tests.
	var table *bigquery.Table
	if *reportTable != "" {
		table, err = prepareTable(ctx, client, *reportTable, *createTable)
		if err != nil {
			log.Fatalf("prepareTable: %v", err)
		}
	}
	start := time.Now()
	profiles, err := runBenchmarks(ctx, client, *queryfile, &tags, *reruns)
	if err != nil {
		log.Fatalf("runBenchmarks: %v", err)
	}
	fmt.Printf("measurement time: %v\n\n", time.Now().Sub(start))
	if table != nil {
		if err := reportResults(ctx, client, table, profiles); err != nil {
			log.Fatalf("reportResults: %v", err)
		}
	}
	printResults(profiles)
}
