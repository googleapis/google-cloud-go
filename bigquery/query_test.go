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

	"golang.org/x/net/context"

	bq "google.golang.org/api/bigquery/v2"
)

func defaultQueryJob() *bq.Job {
	return &bq.Job{
		Configuration: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				DestinationTable: &bq.TableReference{
					ProjectId: "project-id",
					DatasetId: "dataset-id",
					TableId:   "table-id",
				},
				Query: "query string",
				DefaultDataset: &bq.DatasetReference{
					ProjectId: "def-project-id",
					DatasetId: "def-dataset-id",
				},
			},
		},
	}
}

func TestQueryWithOptions(t *testing.T) {
	s := &testService{}
	c := &Client{
		projectID: "project-id",
		service:   s,
	}
	testCases := []struct {
		dst     *Table
		src     *QueryConfig
		options []Option
		want    *bq.Job
	}{
		{
			dst: &Table{
				ProjectID: "project-id",
				DatasetID: "dataset-id",
				TableID:   "table-id",
			},
			src:     defaultQuery,
			options: []Option{CreateNever, WriteTruncate},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.WriteDisposition = "WRITE_TRUNCATE"
				j.Configuration.Query.CreateDisposition = "CREATE_NEVER"
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{DisableQueryCache()},
			want: func() *bq.Job {
				j := defaultQueryJob()
				f := false
				j.Configuration.Query.UseQueryCache = &f
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{AllowLargeResults()},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.AllowLargeResults = true
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{DisableFlattenedResults()},
			want: func() *bq.Job {
				j := defaultQueryJob()
				f := false
				j.Configuration.Query.FlattenResults = &f
				j.Configuration.Query.AllowLargeResults = true
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{JobPriority("low")},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.Priority = "low"
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{MaxBillingTier(3), MaxBytesBilled(5)},
			want: func() *bq.Job {
				j := defaultQueryJob()
				tier := int64(3)
				j.Configuration.Query.MaximumBillingTier = &tier
				j.Configuration.Query.MaximumBytesBilled = 5
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{MaxBytesBilled(-1)},
			want:    defaultQueryJob(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			src:     defaultQuery,
			options: []Option{QueryUseStandardSQL()},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.UseLegacySql = false
				j.Configuration.Query.ForceSendFields = []string{"UseLegacySql"}
				return j
			}(),
		},
	}

	for _, tc := range testCases {
		// Only the old-style Client.Copy method can take options.
		if _, err := c.Copy(context.Background(), tc.dst, tc.src, tc.options...); err != nil {
			t.Errorf("err calling query: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}
	}
}

func TestQuery(t *testing.T) {
	c := &Client{
		projectID: "project-id",
	}
	testCases := []struct {
		dst  *Table
		src  *QueryConfig
		want *bq.Job
	}{
		{
			dst:  c.Dataset("dataset-id").Table("table-id"),
			src:  defaultQuery,
			want: defaultQueryJob(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q: "query string",
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DefaultDataset = nil
				return j
			}(),
		},
		{
			dst: &Table{},
			src: defaultQuery,
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DestinationTable = nil
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: &QueryConfig{
				Q: "query string",
				TableDefinitions: map[string]ExternalData{
					"atable": &GCSReference{
						uris:                []string{"uri"},
						AllowJaggedRows:     true,
						AllowQuotedNewlines: true,
						Compression:         Gzip,
						Encoding:            UTF_8,
						FieldDelimiter:      ";",
						IgnoreUnknownValues: true,
						MaxBadRecords:       1,
						Quote:               "'",
						SkipLeadingRows:     2,
						Schema: Schema([]*FieldSchema{
							{Name: "name", Type: StringFieldType},
						}),
					},
				},
			},
			want: func() *bq.Job {
				j := defaultQueryJob()
				j.Configuration.Query.DefaultDataset = nil
				td := make(map[string]bq.ExternalDataConfiguration)
				quote := "'"
				td["atable"] = bq.ExternalDataConfiguration{
					Compression:         "GZIP",
					IgnoreUnknownValues: true,
					MaxBadRecords:       1,
					SourceFormat:        "CSV", // must be explicitly set.
					SourceUris:          []string{"uri"},
					CsvOptions: &bq.CsvOptions{
						AllowJaggedRows:     true,
						AllowQuotedNewlines: true,
						Encoding:            "UTF-8",
						FieldDelimiter:      ";",
						SkipLeadingRows:     2,
						Quote:               &quote,
					},
					Schema: &bq.TableSchema{
						Fields: []*bq.TableFieldSchema{
							{Name: "name", Type: "STRING"},
						},
					},
				}
				j.Configuration.Query.TableDefinitions = td
				return j
			}(),
		},
	}

	for _, tc := range testCases {
		// Old-style: Client.Copy.
		s := &testService{}
		c.service = s
		if _, err := c.Copy(context.Background(), tc.dst, tc.src); err != nil {
			t.Errorf("err calling query: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}

		// New-style: Client.Query.Run.
		s = &testService{}
		c.service = s
		query := c.Query("")
		query.QueryConfig = *tc.src
		query.Dst = tc.dst
		if _, err := query.Run(context.Background()); err != nil {
			t.Errorf("err calling query: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}
	}
}

func TestConfiguringQuery(t *testing.T) {
	s := &testService{}
	c := &Client{
		projectID: "project-id",
		service:   s,
	}

	query := c.Query("q")
	query.JobID = "ajob"
	query.DefaultProjectID = "def-project-id"
	query.DefaultDatasetID = "def-dataset-id"
	// Note: Other configuration fields are tested in other tests above.
	// A lot of that can be consolidated once Client.Copy is gone.

	want := &bq.Job{
		Configuration: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				Query: "q",
				DefaultDataset: &bq.DatasetReference{
					ProjectId: "def-project-id",
					DatasetId: "def-dataset-id",
				},
			},
		},
		JobReference: &bq.JobReference{
			JobId:     "ajob",
			ProjectId: "project-id",
		},
	}

	if _, err := query.Run(context.Background()); err != nil {
		t.Fatalf("err calling Query.Run: %v", err)
	}
	if !reflect.DeepEqual(s.Job, want) {
		t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, want)
	}
}

func TestDeprecatedFields(t *testing.T) {
	// TODO(jba): delete this test once the deprecated top-level Query fields (e.g. "Q") have been removed.  TestConfiguringQuery will suffice then.
	s := &testService{}
	c := &Client{
		projectID: "project-id",
		service:   s,
	}

	query := c.Query("original query")
	query.QueryConfig.DefaultProjectID = "original project id"
	query.QueryConfig.DefaultDatasetID = "original dataset id"

	// Set deprecated fields.  Should override the one in QueryConfig.
	query.Q = "override query"
	query.DefaultProjectID = "override project id"
	query.DefaultDatasetID = "override dataset id"

	want := &bq.Job{
		Configuration: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				Query: "override query",
				DefaultDataset: &bq.DatasetReference{
					ProjectId: "override project id",
					DatasetId: "override dataset id",
				},
			},
		},
	}

	if _, err := query.Run(context.Background()); err != nil {
		t.Fatalf("err calling Query.Run: %v", err)
	}
	if !reflect.DeepEqual(s.Job, want) {
		t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, want)
	}

	// Clear deprecated fields.  The ones in QueryConfig should now be used.
	query.Q = ""
	query.DefaultProjectID = ""
	query.DefaultDatasetID = ""

	want = &bq.Job{
		Configuration: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				Query: "original query",
				DefaultDataset: &bq.DatasetReference{
					ProjectId: "original project id",
					DatasetId: "original dataset id",
				},
			},
		},
	}

	if _, err := query.Run(context.Background()); err != nil {
		t.Fatalf("err calling Query.Run: %v", err)
	}
	if !reflect.DeepEqual(s.Job, want) {
		t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, want)
	}

}

func TestBackwardsCompatabilityOfQuery(t *testing.T) {
	// TODO(jba): delete this test once Queries can only be created via Client.Query.
	c := &Client{
		projectID: "project-id",
	}
	testCases := []struct {
		src  interface{}
		want *bq.Job
	}{
		{
			src: &Query{
				Q:                "query string",
				DefaultProjectID: "def-project-id",
				DefaultDatasetID: "def-dataset-id",
				TableDefinitions: map[string]ExternalData{"a": c.NewGCSReference("uri")},
			},
			want: &bq.Job{
				Configuration: &bq.JobConfiguration{
					Query: &bq.JobConfigurationQuery{
						Query: "query string",
						DefaultDataset: &bq.DatasetReference{
							ProjectId: "def-project-id",
							DatasetId: "def-dataset-id",
						},
						TableDefinitions: map[string]bq.ExternalDataConfiguration{
							"a": bq.ExternalDataConfiguration{
								CsvOptions:   &bq.CsvOptions{},
								SourceFormat: "CSV",
								SourceUris:   []string{"uri"},
							},
						},
					},
				},
			},
		},
		{
			src: &QueryConfig{
				Q:                "query string",
				DefaultProjectID: "def-project-id",
				DefaultDatasetID: "def-dataset-id",
				TableDefinitions: map[string]ExternalData{"a": c.NewGCSReference("uri")},
			},
			want: &bq.Job{
				Configuration: &bq.JobConfiguration{
					Query: &bq.JobConfigurationQuery{
						Query: "query string",
						DefaultDataset: &bq.DatasetReference{
							ProjectId: "def-project-id",
							DatasetId: "def-dataset-id",
						},
						TableDefinitions: map[string]bq.ExternalDataConfiguration{
							"a": bq.ExternalDataConfiguration{
								CsvOptions:   &bq.CsvOptions{},
								SourceFormat: "CSV",
								SourceUris:   []string{"uri"},
							},
						},
					},
				},
			},
		},
		{
			src: &Query{
				QueryConfig: QueryConfig{
					Q:                "query string",
					DefaultProjectID: "def-project-id",
					DefaultDatasetID: "def-dataset-id",
					TableDefinitions: map[string]ExternalData{"a": c.NewGCSReference("uri")},
				},
			},
			want: &bq.Job{
				Configuration: &bq.JobConfiguration{
					Query: &bq.JobConfigurationQuery{
						Query: "query string",
						DefaultDataset: &bq.DatasetReference{
							ProjectId: "def-project-id",
							DatasetId: "def-dataset-id",
						},
						TableDefinitions: map[string]bq.ExternalDataConfiguration{
							"a": bq.ExternalDataConfiguration{
								CsvOptions:   &bq.CsvOptions{},
								SourceFormat: "CSV",
								SourceUris:   []string{"uri"},
							},
						},
					},
				},
			},
		},
		{
			src: func() *Query {
				q := c.Query("query string")
				q.DefaultProjectID = "def-project-id"
				q.DefaultDatasetID = "def-dataset-id"
				q.TableDefinitions = map[string]ExternalData{"a": c.NewGCSReference("uri")}
				return q
			}(),
			want: &bq.Job{
				Configuration: &bq.JobConfiguration{
					Query: &bq.JobConfigurationQuery{
						Query: "query string",
						DefaultDataset: &bq.DatasetReference{
							ProjectId: "def-project-id",
							DatasetId: "def-dataset-id",
						},
						TableDefinitions: map[string]bq.ExternalDataConfiguration{
							"a": bq.ExternalDataConfiguration{
								CsvOptions:   &bq.CsvOptions{},
								SourceFormat: "CSV",
								SourceUris:   []string{"uri"},
							},
						},
					},
				},
			},
		},
		{
			src: func() *Query {
				q := c.Query("query string")
				q.QueryConfig.DefaultProjectID = "def-project-id"
				q.QueryConfig.DefaultDatasetID = "def-dataset-id"
				q.QueryConfig.TableDefinitions = map[string]ExternalData{"a": c.NewGCSReference("uri")}
				return q
			}(),
			want: &bq.Job{
				Configuration: &bq.JobConfiguration{
					Query: &bq.JobConfigurationQuery{
						Query: "query string",
						DefaultDataset: &bq.DatasetReference{
							ProjectId: "def-project-id",
							DatasetId: "def-dataset-id",
						},
						TableDefinitions: map[string]bq.ExternalDataConfiguration{
							"a": bq.ExternalDataConfiguration{
								CsvOptions:   &bq.CsvOptions{},
								SourceFormat: "CSV",
								SourceUris:   []string{"uri"},
							},
						},
					},
				},
			},
		},
	}

	dst := &Table{}
	for _, tc := range testCases {
		// Old-style: Client.Copy.
		s := &testService{}
		c.service = s
		if _, err := c.Copy(context.Background(), dst, tc.src.(Source)); err != nil {
			t.Errorf("err calling query: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}

		// Old-style Client.Read.
		s = &testService{}
		c.service = s
		if _, err := c.Read(context.Background(), tc.src.(ReadSource)); err != nil {
			t.Errorf("err calling query: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("querying: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}
	}
}
