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

func defaultLoadJob() *bq.Job {
	return &bq.Job{
		Configuration: &bq.JobConfiguration{
			Load: &bq.JobConfigurationLoad{
				DestinationTable: &bq.TableReference{
					ProjectId: "project-id",
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

func TestLoadWithOptions(t *testing.T) {
	c := &Client{projectID: "project-id"}

	testCases := []struct {
		dst     *Table
		src     *GCSReference
		options []Option
		want    *bq.Job
	}{
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: c.NewGCSReference("uri"),
			options: []Option{
				MaxBadRecords(1),
				AllowJaggedRows(),
				AllowQuotedNewlines(),
				IgnoreUnknownValues(),
			},
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.MaxBadRecords = 1
				j.Configuration.Load.AllowJaggedRows = true
				j.Configuration.Load.AllowQuotedNewlines = true
				j.Configuration.Load.IgnoreUnknownValues = true
				return j
			}(),
		},
		{
			dst:     c.Dataset("dataset-id").Table("table-id"),
			options: []Option{CreateNever, WriteTruncate},
			src:     c.NewGCSReference("uri"),
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.CreateDisposition = "CREATE_NEVER"
				j.Configuration.Load.WriteDisposition = "WRITE_TRUNCATE"
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: c.NewGCSReference("uri"),
			options: []Option{
				DestinationSchema(Schema{
					stringFieldSchema(),
					nestedFieldSchema(),
				}),
			},
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
	}
	for _, tc := range testCases {
		s := &testService{}
		c.service = s

		// Only the old-style Client.Copy method can take options.
		if _, err := c.Copy(context.Background(), tc.dst, tc.src, tc.options...); err != nil {
			t.Errorf("err calling load: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("loading: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}
	}
}

func TestLoad(t *testing.T) {
	c := &Client{projectID: "project-id"}

	testCases := []struct {
		dst     *Table
		src     *GCSReference
		options []Option
		want    *bq.Job
	}{
		{
			dst:  c.Dataset("dataset-id").Table("table-id"),
			src:  c.NewGCSReference("uri"),
			want: defaultLoadJob(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := c.NewGCSReference("uri")
				g.MaxBadRecords = 1
				g.AllowJaggedRows = true
				g.AllowQuotedNewlines = true
				g.IgnoreUnknownValues = true
				return g
			}(),
			want: func() *bq.Job {
				j := defaultLoadJob()
				j.Configuration.Load.MaxBadRecords = 1
				j.Configuration.Load.AllowJaggedRows = true
				j.Configuration.Load.AllowQuotedNewlines = true
				j.Configuration.Load.IgnoreUnknownValues = true
				return j
			}(),
		},
		{
			dst: c.Dataset("dataset-id").Table("table-id"),
			src: func() *GCSReference {
				g := c.NewGCSReference("uri")
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
				g := c.NewGCSReference("uri")
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
			src: c.NewGCSReference("uri"),
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
				g := c.NewGCSReference("uri")
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
	}

	for _, tc := range testCases {
		// Old-style: Client.Copy.
		s := &testService{}
		c.service = s
		if _, err := c.Copy(context.Background(), tc.dst, tc.src, tc.options...); err != nil {
			t.Errorf("err calling load: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("loading: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}

		// New-style: Table.LoaderFrom.
		s = &testService{}
		c.service = s
		loader := tc.dst.LoaderFrom(tc.src)
		if _, err := loader.Run(context.Background()); err != nil {
			t.Errorf("err calling Loader.Run: %v", err)
			continue
		}
		if !reflect.DeepEqual(s.Job, tc.want) {
			t.Errorf("loading: got:\n%v\nwant:\n%v", s.Job, tc.want)
		}
	}
}

func TestConfiguringLoader(t *testing.T) {
	s := &testService{}
	c := &Client{
		projectID: "project-id",
		service:   s,
	}

	dst := c.Dataset("dataset-id").Table("table-id")
	src := c.NewGCSReference("uri")

	want := defaultLoadJob()
	want.Configuration.Load.CreateDisposition = "CREATE_NEVER"
	want.Configuration.Load.WriteDisposition = "WRITE_TRUNCATE"

	loader := dst.LoaderFrom(src)
	loader.TableCreateDisposition = CreateNever
	loader.TableWriteDisposition = WriteTruncate

	if _, err := loader.Run(context.Background()); err != nil {
		t.Errorf("err calling Loader.Run: %v", err)
		return
	}
	if !reflect.DeepEqual(s.Job, want) {
		t.Errorf("loading: got:\n%v\nwant:\n%v", s.Job, want)
	}
}
