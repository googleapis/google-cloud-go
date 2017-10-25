// Copyright 2016 Google Inc. All Rights Reserved.
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
	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
)

// ExtractConfig holds the configuration for an extract job.
type ExtractConfig struct {
	// Src is the table from which data will be extracted.
	Src *Table

	// Dst is the destination into which the data will be extracted.
	Dst *GCSReference

	// DisableHeader disables the printing of a header row in exported data.
	DisableHeader bool
}

func (e *ExtractConfig) toBQ() *bq.JobConfigurationExtract {
	var printHeader *bool
	if e.DisableHeader {
		f := false
		printHeader = &f
	}
	return &bq.JobConfigurationExtract{
		DestinationUris:   append([]string{}, e.Dst.URIs...),
		Compression:       string(e.Dst.Compression),
		DestinationFormat: string(e.Dst.DestinationFormat),
		FieldDelimiter:    e.Dst.FieldDelimiter,
		SourceTable:       e.Src.tableRefProto(),
		PrintHeader:       printHeader,
	}
}

func bqToExtractConfig(q *bq.JobConfigurationExtract, c *Client) *ExtractConfig {
	return &ExtractConfig{
		Dst: &GCSReference{
			URIs:              q.DestinationUris,
			Compression:       Compression(q.Compression),
			DestinationFormat: DataFormat(q.DestinationFormat),
			FileConfig: FileConfig{
				CSVOptions: CSVOptions{
					FieldDelimiter: q.FieldDelimiter,
				},
			},
		},
		DisableHeader: q.PrintHeader != nil && !*q.PrintHeader,
		Src:           convertTableReference(q.SourceTable, c),
	}
}

// An Extractor extracts data from a BigQuery table into Google Cloud Storage.
type Extractor struct {
	JobIDConfig
	ExtractConfig
	c *Client
}

// ExtractorTo returns an Extractor which can be used to extract data from a
// BigQuery table into Google Cloud Storage.
// The returned Extractor may optionally be further configured before its Run method is called.
func (t *Table) ExtractorTo(dst *GCSReference) *Extractor {
	return &Extractor{
		c: t.c,
		ExtractConfig: ExtractConfig{
			Src: t,
			Dst: dst,
		},
	}
}

// Run initiates an extract job.
func (e *Extractor) Run(ctx context.Context) (*Job, error) {
	return e.c.insertJob(ctx, e.newJob(), nil)
}

func (e *Extractor) newJob() *bq.Job {
	return &bq.Job{
		JobReference: e.JobIDConfig.createJobRef(e.c.projectID),
		Configuration: &bq.JobConfiguration{
			Extract: e.ExtractConfig.toBQ(),
		},
	}
}
