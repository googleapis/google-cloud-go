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

package bigquery

import (
	"reflect"
	"testing"

	bq "google.golang.org/api/bigquery/v2"
)

func defaultExtractJob() *bq.Job {
	return &bq.Job{
		Configuration: &bq.JobConfiguration{
			Extract: &bq.JobConfigurationExtract{
				SourceTable: &bq.TableReference{
					ProjectId: "project-id",
					DatasetId: "dataset-id",
					TableId:   "table-id",
				},
				DestinationUris: []string{"uri"},
			},
		},
	}
}

func TestExtract(t *testing.T) {
	testCases := []struct {
		dst     *GCSReference
		src     *Table
		options []Option
		want    *bq.Job
	}{
		{
			dst:  defaultGCS,
			src:  defaultTable,
			want: defaultExtractJob(),
		},
		{
			dst: defaultGCS,
			src: defaultTable,
			options: []Option{
				DisableHeader(),
			},
			want: func() *bq.Job {
				j := defaultExtractJob()
				j.Configuration.Extract.PrintHeader = false
				return j
			}(),
		},
		{
			dst: &GCSReference{
				uris:              []string{"uri"},
				Compression:       Gzip,
				DestinationFormat: JSON,
				FieldDelimiter:    "\t",
			},
			src: defaultTable,
			want: func() *bq.Job {
				j := defaultExtractJob()
				j.Configuration.Extract.Compression = "GZIP"
				j.Configuration.Extract.DestinationFormat = "NEWLINE_DELIMITED_JSON"
				j.Configuration.Extract.FieldDelimiter = "\t"
				return j
			}(),
		},
	}

	for _, tc := range testCases {
		c := &testClient{}
		if _, err := extract(tc.dst, tc.src, c, tc.options); err != nil {
			t.Errorf("err calling extract: %v", err)
			continue
		}
		if !reflect.DeepEqual(c.Job, tc.want) {
			t.Errorf("insertJob got:\n%v\nwant:\n%v", c.Job, tc.want)
		}
	}
}
