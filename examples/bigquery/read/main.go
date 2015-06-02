// Copyright 2015 Google Inc. All Rights Reserved.
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

// read is an example client of the bigquery client library.
// It reads from a table, returning the data via an Iterator.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud/bigquery"
)

var (
	project = flag.String("project", "", "The ID of a Google Cloud Platform project")
	dataset = flag.String("dataset", "", "The ID of a BigQuery dataset")
	table   = flag.String("table", "", "The ID of a BigQuery table.")
)

func main() {
	flag.Parse()

	flagsOk := true
	for _, f := range []string{"project", "dataset", "table"} {
		if flag.Lookup(f).Value.String() == "" {
			fmt.Fprintf(os.Stderr, "Flag --%s is required\n", f)
			flagsOk = false
		}
	}
	if !flagsOk {
		os.Exit(1)
	}

	httpClient, err := google.DefaultClient(context.Background(), bigquery.Scope)
	if err != nil {
		log.Fatalf("Creating http client: %v", err)
	}

	client, err := bigquery.NewClient(httpClient, *project)
	if err != nil {
		log.Fatalf("Creating bigquery client: %v", err)
	}

	it, err := client.Read(&bigquery.Table{
		ProjectID: *project,
		DatasetID: *dataset,
		TableID:   *table,
	})

	if err != nil {
		log.Fatalf("Reading: %v", err)
	}

	// one-space padding.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)

	for it.Next(context.Background()) {
		var vals bigquery.ValueList
		if err := it.Get(&vals); err != nil {
			fmt.Printf("err calling get: %v\n", err)
		} else {
			sep := ""
			for _, v := range vals {
				fmt.Fprintf(tw, "%s%v", sep, v)
				sep = "\t"
			}
			fmt.Fprintf(tw, "\n")
		}
	}
	tw.Flush()

	if err := it.Err(); err != nil {
		fmt.Printf("err reading: %v\n")
	}
}
