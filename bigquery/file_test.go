// Copyright 2016 Google LLC
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
	"testing"

	"cloud.google.com/go/internal/testutil"
	bq "google.golang.org/api/bigquery/v2"
)

var (
	hyphen = "-"
	fc     = FileConfig{
		SourceFormat:        CSV,
		AutoDetect:          true,
		MaxBadRecords:       7,
		IgnoreUnknownValues: true,
		Schema: Schema{
			stringFieldSchema(),
			nestedFieldSchema(),
		},
		CSVOptions: CSVOptions{
			Quote:                          hyphen,
			FieldDelimiter:                 "\t",
			SkipLeadingRows:                8,
			AllowJaggedRows:                true,
			AllowQuotedNewlines:            true,
			Encoding:                       UTF_8,
			NullMarker:                     "marker",
			PreserveASCIIControlCharacters: true,
		},
	}
)

func TestFileConfigPopulateLoadConfig(t *testing.T) {
	testcases := []struct {
		description string
		fileConfig  *FileConfig
		want        *bq.JobConfigurationLoad
	}{
		{
			description: "default json",
			fileConfig: &FileConfig{
				SourceFormat: JSON,
			},
			want: &bq.JobConfigurationLoad{
				SourceFormat: "NEWLINE_DELIMITED_JSON",
			},
		},
		{
			description: "csv",
			fileConfig:  &fc,
			want: &bq.JobConfigurationLoad{
				SourceFormat:                   "CSV",
				FieldDelimiter:                 "\t",
				SkipLeadingRows:                8,
				AllowJaggedRows:                true,
				AllowQuotedNewlines:            true,
				Autodetect:                     true,
				Encoding:                       "UTF-8",
				MaxBadRecords:                  7,
				IgnoreUnknownValues:            true,
				NullMarker:                     "marker",
				PreserveAsciiControlCharacters: true,
				Schema: &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqStringFieldSchema(),
						bqNestedFieldSchema(),
					}},
				Quote: &hyphen,
			},
		},
		{
			description: "parquet",
			fileConfig: &FileConfig{
				SourceFormat: Parquet,
				ParquetOptions: &ParquetOptions{
					EnumAsString:        true,
					EnableListInference: true,
				},
			},
			want: &bq.JobConfigurationLoad{
				SourceFormat: "PARQUET",
				ParquetOptions: &bq.ParquetOptions{
					EnumAsString:        true,
					EnableListInference: true,
				},
			},
		},
		{
			description: "avro",
			fileConfig: &FileConfig{
				SourceFormat: Avro,
				AvroOptions: &AvroOptions{
					UseAvroLogicalTypes: true,
				},
			},
			want: &bq.JobConfigurationLoad{
				SourceFormat:        "AVRO",
				UseAvroLogicalTypes: true,
			},
		},
	}
	for _, tc := range testcases {
		got := &bq.JobConfigurationLoad{}
		tc.fileConfig.populateLoadConfig(got)
		if diff := testutil.Diff(got, tc.want); diff != "" {
			t.Errorf("case %s, got=-, want=+:\n%s", tc.description, diff)
		}
	}
}

func TestFileConfigPopulateExternalDataConfig(t *testing.T) {
	testcases := []struct {
		description string
		fileConfig  *FileConfig
		want        *bq.ExternalDataConfiguration
	}{
		{
			description: "json defaults",
			fileConfig: &FileConfig{
				SourceFormat: JSON,
			},
			want: &bq.ExternalDataConfiguration{
				SourceFormat: "NEWLINE_DELIMITED_JSON",
			},
		},
		{
			description: "csv fileconfig",
			fileConfig:  &fc,
			want: &bq.ExternalDataConfiguration{
				SourceFormat:        "CSV",
				Autodetect:          true,
				MaxBadRecords:       7,
				IgnoreUnknownValues: true,
				Schema: &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqStringFieldSchema(),
						bqNestedFieldSchema(),
					}},
				CsvOptions: &bq.CsvOptions{
					AllowJaggedRows:                true,
					AllowQuotedNewlines:            true,
					Encoding:                       "UTF-8",
					FieldDelimiter:                 "\t",
					Quote:                          &hyphen,
					SkipLeadingRows:                8,
					NullMarker:                     "marker",
					PreserveAsciiControlCharacters: true,
				},
			},
		},
		{
			description: "parquet",
			fileConfig: &FileConfig{
				SourceFormat: Parquet,
				ParquetOptions: &ParquetOptions{
					EnumAsString:        true,
					EnableListInference: true,
				},
			},
			want: &bq.ExternalDataConfiguration{
				SourceFormat: "PARQUET",
				ParquetOptions: &bq.ParquetOptions{
					EnumAsString:        true,
					EnableListInference: true,
				},
			},
		},
	}
	for _, tc := range testcases {
		got := &bq.ExternalDataConfiguration{}
		tc.fileConfig.populateExternalDataConfig(got)
		if diff := testutil.Diff(got, tc.want); diff != "" {
			t.Errorf("case %s, got=-, want=+:\n%s", tc.description, diff)
		}
	}

}
