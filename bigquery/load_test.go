// Copyright 2015 Google LLC
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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	bq "google.golang.org/api/bigquery/v2"
)

func defaultLoadJob() *bq.Job {
	return &bq.Job{
		JobReference: &bq.JobReference{JobId: "RANDOM", ProjectId: "client-project-id"},
		Configuration: &bq.JobConfiguration{
			Load: &bq.JobConfigurationLoad{
				DestinationTable: &bq.TableReference{
					ProjectId: "client-project-id",
					DatasetId: "dataset-id",
					TableId:   "table-id",
				},
				SourceUris: []string{"uri"},
			},
		},
	}
}

func stringFieldSchema() *FieldSchema {
	return &FieldSchema{Name: "fieldname", Type: StringFieldType}
}

func nestedFieldSchema() *FieldSchema {
	return &FieldSchema{
		Name:   "nested",
		Type:   RecordFieldType,
		Schema: Schema{stringFieldSchema()},
	}
}

func bqStringFieldSchema() *bq.TableFieldSchema {
	return &bq.TableFieldSchema{
		Name: "fieldname",
		Type: "STRING",
	}
}

func bqNestedFieldSchema() *bq.TableFieldSchema {
	return &bq.TableFieldSchema{
		Name:   "nested",
		Type:   "RECORD",
		Fields: []*bq.TableFieldSchema{bqStringFieldSchema()},
	}
}

func TestLoad(t *testing.T) {
	defer fixRandomID("RANDOM")()
	c := &Client{projectID: "client-project-id"}

	testCases := []struct {
		dst      *Table
		src      LoadSource
		jobID    string
		location string
		config   LoadConfig
		want     *bq.Job
	}{
		{
			dst:  c.Dataset("dataset-id").Table("table-id"),
			src:  NewGCSReference("uri"),
			want: defaultLoadJob(),
		},
		{
			dst:      c.Dataset("dataset-id").Table("table-id"),
			src:      NewGCSReference("uri"),
			location: "loc",
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.JobReference.Location = "loc"
				return j
			}(),
		},
		{
			dst:   c.Dataset("dataset-id").Table("table-id"),
			jobID: "ajob",
			config: LoadConfig{
				CreateDisposition:           CreateNever,
				WriteDisposition:            WriteTruncate,
				Labels:                      map[string]string{"a": "b"},
				TimePartitioning:            &TimePartitioning{Type: MonthPartitioningType, Expiration: 1234 * time.Millisecond},
				Clustering:                  &Clustering{Fields: []string{"cfield1"}},
				DestinationEncryptionConfig: &EncryptionConfig{KMSKeyName: "keyName"},
				SchemaUpdateOptions:         []string{"ALLOW_FIELD_ADDITION"},
			},
			src: NewGCSReference("uri"),
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Labels = map[string]string{"a": "b"}
				j.Configuration.Load.CreateDisposition = "CREATE_NEVER"
				j.Configuration.Load.WriteDisposition = "WRITE_TRUNCATE"
				j.Configuration.Load.TimePartitioning = &bq.TimePartitioning{
					Type:         "MONTH",
					ExpirationMs: 1234,
				}
				j.Configuration.Load.Clustering = &bq.Clustering{
					Fields: []string{"cfield1"},
				}
				j.Configuration.Load.DestinationEncryptionConfiguration = &bq.EncryptionConfiguration{KmsKeyName: "keyName"}
				j.JobReference = &bq.JobReference{
					JobId:     "ajob",
					ProjectId: "client-project-id",
				}
				j.Configuration.Load.SchemaUpdateOptions = []string{"ALLOW_FIELD_ADDITION"}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.MaxBadRecords = 1
				g.AllowJaggedRows = true
				g.AllowQuotedNewlines = true
				g.IgnoreUnknownValues = true
				return g
			}(),
			config: LoadConfig{
				JobTimeout: 4 * time.Second,
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.MaxBadRecords = 1
				j.Configuration.Load.AllowJaggedRows = true
				j.Configuration.Load.AllowQuotedNewlines = true
				j.Configuration.Load.IgnoreUnknownValues = true
				j.Configuration.JobTimeoutMs = 4000
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.Schema = Schema{
					stringFieldSchema(),
					nestedFieldSchema(),
				}
				return g
			}(),
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.Schema = &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqStringFieldSchema(),
						bqNestedFieldSchema(),
					}}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.SkipLeadingRows = 1
				g.SourceFormat = JSON
				g.Encoding = UTF_8
				g.FieldDelimiter = "\t"
				g.Quote = "-"
				return g
			}(),
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SkipLeadingRows = 1
				j.Configuration.Load.SourceFormat = "NEWLINE_DELIMITED_JSON"
				j.Configuration.Load.Encoding = "UTF-8"
				j.Configuration.Load.FieldDelimiter = "\t"
				hyphen := "-"
				j.Configuration.Load.Quote = &hyphen
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: NewGCSReference("uri"),
			want: func() *bq.Job {
				j := defaultLoadJob()
				// Quote is left unset in GCSReference, so should be nil here.
				j.Configuration.Load.Quote = nil
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.ForceZeroQuote = true
				return g
			}(),
			want: func() *bq.Job {
				j := defaultLoadJob()
				empty := ""
				j.Configuration.Load.Quote = &empty
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *ReaderSource {
				r := NewReaderSource(strings.NewReader("foo"))
				r.SkipLeadingRows = 1
				r.SourceFormat = JSON
				r.Encoding = UTF_8
				r.FieldDelimiter = "\t"
				r.Quote = "-"
				return r
			}(),
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceUris = nil
				j.Configuration.Load.SkipLeadingRows = 1
				j.Configuration.Load.SourceFormat = "NEWLINE_DELIMITED_JSON"
				j.Configuration.Load.Encoding = "UTF-8"
				j.Configuration.Load.FieldDelimiter = "\t"
				hyphen := "-"
				j.Configuration.Load.Quote = &hyphen
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.SourceFormat = Avro
				return g
			}(),
			config: LoadConfig{
				UseAvroLogicalTypes: true,
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceFormat = "AVRO"
				j.Configuration.Load.UseAvroLogicalTypes = true
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *ReaderSource {
				r := NewReaderSource(strings.NewReader("foo"))
				r.SourceFormat = Avro
				return r
			}(),
			config: LoadConfig{
				UseAvroLogicalTypes: true,
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceUris = nil
				j.Configuration.Load.SourceFormat = "AVRO"
				j.Configuration.Load.UseAvroLogicalTypes = true
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *ReaderSource {
				r := NewReaderSource(strings.NewReader("foo"))
				return r
			}(),
			config: LoadConfig{
				TimePartitioning: &TimePartitioning{
					Type:  HourPartitioningType,
					Field: "somefield",
				},
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceUris = nil
				j.Configuration.Load.TimePartitioning = &bq.TimePartitioning{
					Field: "somefield",
					Type:  "HOUR",
				}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *ReaderSource {
				r := NewReaderSource(strings.NewReader("foo"))
				return r
			}(),
			config: LoadConfig{
				RangePartitioning: &RangePartitioning{
					Field: "somefield",
					Range: &RangePartitioningRange{
						Start:    1,
						End:      2,
						Interval: 3,
					},
				},
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceUris = nil
				j.Configuration.Load.RangePartitioning = &bq.RangePartitioning{
					Field: "somefield",
					Range: &bq.RangePartitioningRange{
						Start:           1,
						End:             2,
						Interval:        3,
						ForceSendFields: []string{"Start", "End", "Interval"},
					},
				}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.SourceFormat = DatastoreBackup
				return g
			}(),
			config: LoadConfig{
				ProjectionFields: []string{"foo", "bar", "baz"},
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceFormat = "DATASTORE_BACKUP"
				j.Configuration.Load.ProjectionFields = []string{"foo", "bar", "baz"}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.SourceFormat = Parquet
				return g
			}(),
			config: LoadConfig{
				HivePartitioningOptions: &HivePartitioningOptions{
					Mode:                   CustomHivePartitioningMode,
					SourceURIPrefix:        "source_uri",
					RequirePartitionFilter: true,
				},
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceFormat = "PARQUET"
				j.Configuration.Load.HivePartitioningOptions = &bq.HivePartitioningOptions{
					Mode:                   "CUSTOM",
					RequirePartitionFilter: true,
					SourceUriPrefix:        "source_uri",
				}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.SourceFormat = Parquet
				return g
			}(),
			config: LoadConfig{
				DecimalTargetTypes: []DecimalTargetType{BigNumericTargetType, NumericTargetType, StringTargetType},
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceFormat = "PARQUET"
				j.Configuration.Load.DecimalTargetTypes = []string{"BIGNUMERIC", "NUMERIC", "STRING"}
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				g.SourceFormat = Parquet
				return g
			}(),
			config: LoadConfig{
				ReferenceFileSchemaURI: "schema.parquet",
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.SourceFormat = "PARQUET"
				j.Configuration.Load.ReferenceFileSchemaUri = "schema.parquet"
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := NewGCSReference("uri")
				return g
			}(),
			config: LoadConfig{
				CreateSession: true,
				ConnectionProperties: []*ConnectionProperty{
					{
						Key:   "session_id",
						Value: "session_id_1234567890",
					},
				},
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.CreateSession = true
				j.Configuration.Load.ConnectionProperties = []*bq.ConnectionProperty{
					{
						Key:   "session_id",
						Value: "session_id_1234567890",
					},
				}
				return j
			}(),
		},
	}

	for i, tc := range testCases {
		loader := tc.dst.LoaderFrom(tc.src)
		loader.JobID = tc.jobID
		loader.Location = tc.location
		tc.config.Src = tc.src
		tc.config.Dst = tc.dst
		loader.LoadConfig = tc.config
		got, _ := loader.newJob()
		checkJob(t, i, got, tc.want)

		jc, err := bqToJobConfig(got.Configuration, c)
		if err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
		diff := testutil.Diff(jc.(*LoadConfig), &loader.LoadConfig,
			cmp.AllowUnexported(Table{}, Client{}),
			cmpopts.IgnoreUnexported(ReaderSource{}))
		if diff != "" {
			t.Errorf("#%d: (got=-, want=+:\n%s", i, diff)
		}
	}
}
