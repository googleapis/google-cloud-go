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
	"regexp"
	"strings"
	"text/tabwriter"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud/bigquery"
)

var (
	project = flag.String("project", "", "The ID of a Google Cloud Platform project")
	dataset = flag.String("dataset", "", "The ID of a BigQuery dataset")
	table   = flag.String("table", ".*", "A regular expression to match the IDs of tables to read.")
)

func printTable(client *bigquery.Client, t *bigquery.Table) {
	it, err := client.Read(context.Background(), t)

	if err != nil {
		log.Fatalf("Reading: %v", err)
	}

	id := t.FullyQualifiedName()
	fmt.Printf("%s\n%s\n", id, strings.Repeat("-", len(id)))

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

	fmt.Printf("\n")
	if err := it.Err(); err != nil {
		fmt.Printf("err reading: %v\n")
	}
}

func main() {
	flag.Parse()

	flagsOk := true
	for _, f := range []string{"project", "dataset"} {
		if flag.Lookup(f).Value.String() == "" {
			fmt.Fprintf(os.Stderr, "Flag --%s is required\n", f)
			flagsOk = false
		}
	}
	if !flagsOk {
		os.Exit(1)
	}

	tableRE, err := regexp.Compile(*table)
	if err != nil {
		fmt.Fprintf(os.Stderr, "--table is not a valid regular expression: %q\n", *table)
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

	ds := client.Dataset(*dataset)
	var tables []*bigquery.Table
	tables, err = ds.ListTables(context.Background())
	if err != nil {
		log.Fatalf("Listing tables: %v", err)
	}
	for _, t := range tables {
		if tableRE.MatchString(t.TableID) {
			printTable(client, t)
		}
	}
}
