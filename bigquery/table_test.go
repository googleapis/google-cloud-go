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

package bigquery

import (
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

func TestBQToTableMetadata(t *testing.T) {
	bqClient := &Client{}
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	aTimeMillis := aTime.UnixNano() / 1e6
	aDurationMillis := int64(1800000)
	aDuration := time.Duration(aDurationMillis) * time.Millisecond
	aStalenessValue, _ := ParseInterval("8:0:0")
	for _, test := range []struct {
		in   *bq.Table
		want *TableMetadata
	}{
		{&bq.Table{}, &TableMetadata{}}, // test minimal case
		{
			&bq.Table{
				CreationTime:     aTimeMillis,
				Description:      "desc",
				Etag:             "etag",
				ExpirationTime:   aTimeMillis,
				FriendlyName:     "fname",
				Id:               "id",
				LastModifiedTime: uint64(aTimeMillis),
				Location:         "loc",
				NumBytes:         123,
				NumLongTermBytes: 23,
				NumRows:          7,
				StreamingBuffer: &bq.Streamingbuffer{
					EstimatedBytes:  11,
					EstimatedRows:   3,
					OldestEntryTime: uint64(aTimeMillis),
				},
				MaterializedView: &bq.MaterializedViewDefinition{
					EnableRefresh:                 true,
					Query:                         "mat view query",
					LastRefreshTime:               aTimeMillis,
					RefreshIntervalMs:             aDurationMillis,
					AllowNonIncrementalDefinition: true,
					MaxStaleness:                  "8:0:0",
				},
				TimePartitioning: &bq.TimePartitioning{
					ExpirationMs: 7890,
					Type:         "DAY",
					Field:        "pfield",
				},
				Clustering: &bq.Clustering{
					Fields: []string{"cfield1", "cfield2"},
				},
				RequirePartitionFilter:  true,
				EncryptionConfiguration: &bq.EncryptionConfiguration{KmsKeyName: "keyName"},
				Type:                    "EXTERNAL",
				View:                    &bq.ViewDefinition{Query: "view-query"},
				Labels:                  map[string]string{"a": "b"},
				ExternalDataConfiguration: &bq.ExternalDataConfiguration{
					SourceFormat: "GOOGLE_SHEETS",
				},
				TableConstraints: &bq.TableConstraints{
					PrimaryKey: &bq.TableConstraintsPrimaryKey{
						Columns: []string{"id"},
					},
					ForeignKeys: []*bq.TableConstraintsForeignKeys{
						{
							Name: "fk",
							ColumnReferences: []*bq.TableConstraintsForeignKeysColumnReferences{
								{
									ReferencedColumn:  "id",
									ReferencingColumn: "parent",
								},
							},
							ReferencedTable: &bq.TableConstraintsForeignKeysReferencedTable{
								DatasetId: "dataset_id",
								ProjectId: "project_id",
								TableId:   "table_id",
							},
						},
					},
				},
				ResourceTags: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			&TableMetadata{
				Description:        "desc",
				Name:               "fname",
				Location:           "loc",
				ViewQuery:          "view-query",
				FullID:             "id",
				Type:               ExternalTable,
				Labels:             map[string]string{"a": "b"},
				ExternalDataConfig: &ExternalDataConfig{SourceFormat: GoogleSheets},
				ExpirationTime:     aTime.Truncate(time.Millisecond),
				CreationTime:       aTime.Truncate(time.Millisecond),
				LastModifiedTime:   aTime.Truncate(time.Millisecond),
				NumBytes:           123,
				NumLongTermBytes:   23,
				NumRows:            7,
				MaterializedView: &MaterializedViewDefinition{
					EnableRefresh:                 true,
					Query:                         "mat view query",
					LastRefreshTime:               aTime,
					RefreshInterval:               aDuration,
					AllowNonIncrementalDefinition: true,
					MaxStaleness:                  aStalenessValue,
				},
				TimePartitioning: &TimePartitioning{
					Type:       DayPartitioningType,
					Expiration: 7890 * time.Millisecond,
					Field:      "pfield",
				},
				Clustering: &Clustering{
					Fields: []string{"cfield1", "cfield2"},
				},
				RequirePartitionFilter: true,
				StreamingBuffer: &StreamingBuffer{
					EstimatedBytes:  11,
					EstimatedRows:   3,
					OldestEntryTime: aTime,
				},
				EncryptionConfig: &EncryptionConfig{KMSKeyName: "keyName"},
				ETag:             "etag",
				TableConstraints: &TableConstraints{
					PrimaryKey: &PrimaryKey{
						Columns: []string{"id"},
					},
					ForeignKeys: []*ForeignKey{
						{
							Name: "fk",
							ReferencedTable: &Table{
								c:         bqClient,
								ProjectID: "project_id",
								DatasetID: "dataset_id",
								TableID:   "table_id",
							},
							ColumnReferences: []*ColumnReference{
								{
									ReferencedColumn:  "id",
									ReferencingColumn: "parent",
								},
							},
						},
					},
				},
				ResourceTags: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
		},
	} {
		got, err := bqToTableMetadata(test.in, bqClient)
		if err != nil {
			t.Fatal(err)
		}
		if diff := testutil.Diff(got, test.want, cmp.AllowUnexported(Client{}, Table{})); diff != "" {
			t.Errorf("%+v:\n, -got, +want:\n%s", test.in, diff)
		}
	}
}

func TestTableMetadataToBQ(t *testing.T) {
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	aTimeMillis := aTime.UnixNano() / 1e6
	sc := Schema{fieldSchema("desc", "name", "STRING", false, true, nil)}

	for _, test := range []struct {
		in   *TableMetadata
		want *bq.Table
	}{
		{nil, &bq.Table{}},
		{&TableMetadata{}, &bq.Table{}},
		{
			&TableMetadata{
				Name:               "n",
				Description:        "d",
				Schema:             sc,
				ExpirationTime:     aTime,
				Labels:             map[string]string{"a": "b"},
				ExternalDataConfig: &ExternalDataConfig{SourceFormat: Bigtable},
				EncryptionConfig:   &EncryptionConfig{KMSKeyName: "keyName"},
				ResourceTags: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			&bq.Table{
				FriendlyName: "n",
				Description:  "d",
				Schema: &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqTableFieldSchema("desc", "name", "STRING", "REQUIRED", nil),
					},
				},
				ExpirationTime:            aTimeMillis,
				Labels:                    map[string]string{"a": "b"},
				ExternalDataConfiguration: &bq.ExternalDataConfiguration{SourceFormat: "BIGTABLE"},
				EncryptionConfiguration:   &bq.EncryptionConfiguration{KmsKeyName: "keyName"},
				ResourceTags: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
		},
		{
			&TableMetadata{ViewQuery: "q"},
			&bq.Table{
				View: &bq.ViewDefinition{
					Query:           "q",
					UseLegacySql:    false,
					ForceSendFields: []string{"UseLegacySql"},
				},
			},
		},
		{
			&TableMetadata{
				ViewQuery:              "q",
				UseLegacySQL:           true,
				TimePartitioning:       &TimePartitioning{},
				RequirePartitionFilter: true,
			},
			&bq.Table{
				View: &bq.ViewDefinition{
					Query:        "q",
					UseLegacySql: true,
				},
				TimePartitioning: &bq.TimePartitioning{
					Type:         "DAY",
					ExpirationMs: 0,
				},
				RequirePartitionFilter: true,
			},
		},
		{
			&TableMetadata{
				ViewQuery:      "q",
				UseStandardSQL: true,
				TimePartitioning: &TimePartitioning{
					Type:       HourPartitioningType,
					Expiration: time.Second,
					Field:      "ofDreams",
				},
				Clustering: &Clustering{
					Fields: []string{"cfield1"},
				},
			},
			&bq.Table{
				View: &bq.ViewDefinition{
					Query:           "q",
					UseLegacySql:    false,
					ForceSendFields: []string{"UseLegacySql"},
				},
				TimePartitioning: &bq.TimePartitioning{
					Type:         "HOUR",
					ExpirationMs: 1000,
					Field:        "ofDreams",
				},
				Clustering: &bq.Clustering{
					Fields: []string{"cfield1"},
				},
			},
		},
		{
			&TableMetadata{
				RangePartitioning: &RangePartitioning{
					Field: "ofNumbers",
					Range: &RangePartitioningRange{
						Start:    1,
						End:      100,
						Interval: 5,
					},
				},
				Clustering: &Clustering{
					Fields: []string{"cfield1"},
				},
			},
			&bq.Table{

				RangePartitioning: &bq.RangePartitioning{
					Field: "ofNumbers",
					Range: &bq.RangePartitioningRange{
						Start:           1,
						End:             100,
						Interval:        5,
						ForceSendFields: []string{"Start", "End", "Interval"},
					},
				},
				Clustering: &bq.Clustering{
					Fields: []string{"cfield1"},
				},
			},
		},
		{
			&TableMetadata{ExpirationTime: NeverExpire},
			&bq.Table{ExpirationTime: 0},
		},
	} {
		got, err := test.in.toBQ()
		if err != nil {
			t.Fatalf("%+v: %v", test.in, err)
		}
		if diff := testutil.Diff(got, test.want); diff != "" {
			t.Errorf("%+v:\n-got, +want:\n%s", test.in, diff)
		}
	}

	// Errors
	for _, in := range []*TableMetadata{
		{Schema: sc, ViewQuery: "q"}, // can't have both schema and query
		{UseLegacySQL: true},         // UseLegacySQL without query
		{UseStandardSQL: true},       // UseStandardSQL without query
		// read-only fields
		{FullID: "x"},
		{Type: "x"},
		{CreationTime: aTime},
		{LastModifiedTime: aTime},
		{NumBytes: 1},
		{NumLongTermBytes: 1},
		{NumRows: 1},
		{StreamingBuffer: &StreamingBuffer{}},
		{ETag: "x"},
		// expiration time outside allowable range is invalid
		// See https://godoc.org/time#Time.UnixNano
		{ExpirationTime: time.Date(1677, 9, 21, 0, 12, 43, 145224192, time.UTC).Add(-1)},
		{ExpirationTime: time.Date(2262, 04, 11, 23, 47, 16, 854775807, time.UTC).Add(1)},
	} {
		_, err := in.toBQ()
		if err == nil {
			t.Errorf("%+v: got nil, want error", in)
		}
	}
}

func TestTableMetadataToUpdateToBQ(t *testing.T) {
	aTime := time.Date(2017, 1, 26, 0, 0, 0, 0, time.Local)
	for _, test := range []struct {
		tm   TableMetadataToUpdate
		want *bq.Table
	}{
		{
			tm:   TableMetadataToUpdate{},
			want: &bq.Table{},
		},
		{
			tm: TableMetadataToUpdate{
				Description: "d",
				Name:        "n",
			},
			want: &bq.Table{
				Description:     "d",
				FriendlyName:    "n",
				ForceSendFields: []string{"Description", "FriendlyName"},
			},
		},
		{
			tm: TableMetadataToUpdate{
				Schema:         Schema{fieldSchema("desc", "name", "STRING", false, true, nil)},
				ExpirationTime: aTime,
			},
			want: &bq.Table{
				Schema: &bq.TableSchema{
					Fields: []*bq.TableFieldSchema{
						bqTableFieldSchema("desc", "name", "STRING", "REQUIRED", nil),
					},
				},
				ExpirationTime:  aTime.UnixNano() / 1e6,
				ForceSendFields: []string{"Schema", "ExpirationTime"},
			},
		},
		{
			tm: TableMetadataToUpdate{ViewQuery: "q"},
			want: &bq.Table{
				View: &bq.ViewDefinition{Query: "q", ForceSendFields: []string{"Query"}},
			},
		},
		{
			tm: TableMetadataToUpdate{UseLegacySQL: false},
			want: &bq.Table{
				View: &bq.ViewDefinition{
					UseLegacySql:    false,
					ForceSendFields: []string{"UseLegacySql"},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{ViewQuery: "q", UseLegacySQL: true},
			want: &bq.Table{
				View: &bq.ViewDefinition{
					Query:           "q",
					UseLegacySql:    true,
					ForceSendFields: []string{"Query", "UseLegacySql"},
				},
			},
		},
		{
			tm: func() (tm TableMetadataToUpdate) {
				tm.SetLabel("L", "V")
				tm.DeleteLabel("D")
				return tm
			}(),
			want: &bq.Table{
				Labels:     map[string]string{"L": "V"},
				NullFields: []string{"Labels.D"},
			},
		},
		{
			tm: TableMetadataToUpdate{ExpirationTime: NeverExpire},
			want: &bq.Table{
				NullFields: []string{"ExpirationTime"},
			},
		},
		{
			tm: TableMetadataToUpdate{TimePartitioning: &TimePartitioning{Expiration: 0}},
			want: &bq.Table{
				TimePartitioning: &bq.TimePartitioning{
					Type:            "DAY",
					ForceSendFields: []string{"RequirePartitionFilter"},
					NullFields:      []string{"ExpirationMs"},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{TimePartitioning: &TimePartitioning{Expiration: time.Duration(time.Hour)}},
			want: &bq.Table{
				TimePartitioning: &bq.TimePartitioning{
					ExpirationMs:    3600000,
					Type:            "DAY",
					ForceSendFields: []string{"RequirePartitionFilter"},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{RequirePartitionFilter: false},
			want: &bq.Table{
				RequirePartitionFilter: false,
				ForceSendFields:        []string{"RequirePartitionFilter"},
			},
		},
		{
			tm: TableMetadataToUpdate{RequirePartitionFilter: true},
			want: &bq.Table{
				RequirePartitionFilter: true,
				ForceSendFields:        []string{"RequirePartitionFilter"},
			},
		},
		{
			tm: TableMetadataToUpdate{Clustering: &Clustering{Fields: []string{"foo", "bar"}}},
			want: &bq.Table{
				Clustering: &bq.Clustering{Fields: []string{"foo", "bar"}},
			},
		},
		{
			tm: TableMetadataToUpdate{
				TableConstraints: &TableConstraints{
					PrimaryKey: &PrimaryKey{
						Columns: []string{"name"},
					},
				},
			},
			want: &bq.Table{
				TableConstraints: &bq.TableConstraints{
					PrimaryKey: &bq.TableConstraintsPrimaryKey{
						Columns:         []string{"name"},
						ForceSendFields: []string{"Columns"},
					},
					ForceSendFields: []string{"PrimaryKey"},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{
				TableConstraints: &TableConstraints{
					ForeignKeys: []*ForeignKey{
						{
							Name: "fk",
							ReferencedTable: &Table{
								ProjectID: "projectID",
								DatasetID: "datasetID",
								TableID:   "tableID",
							},
							ColumnReferences: []*ColumnReference{
								{
									ReferencedColumn:  "id",
									ReferencingColumn: "other_table_id",
								},
							},
						},
					},
				},
			},
			want: &bq.Table{
				TableConstraints: &bq.TableConstraints{
					ForceSendFields: []string{"ForeignKeys"},
					ForeignKeys: []*bq.TableConstraintsForeignKeys{
						{
							Name: "fk",
							ReferencedTable: &bq.TableConstraintsForeignKeysReferencedTable{
								ProjectId: "projectID",
								DatasetId: "datasetID",
								TableId:   "tableID",
							},
							ColumnReferences: []*bq.TableConstraintsForeignKeysColumnReferences{
								{
									ReferencedColumn:  "id",
									ReferencingColumn: "other_table_id",
								},
							},
						},
					},
				},
			},
		},
		{
			tm: TableMetadataToUpdate{
				ResourceTags: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			want: &bq.Table{
				ResourceTags: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
				ForceSendFields: []string{"ResourceTags"},
			},
		},
	} {
		got, _ := test.tm.toBQ()
		if !testutil.Equal(got, test.want) {
			t.Errorf("%+v:\ngot  %+v\nwant %+v", test.tm, got, test.want)
		}
	}
}

func TestTableMetadataToUpdateToBQErrors(t *testing.T) {
	// See https://godoc.org/time#Time.UnixNano
	start := time.Date(1677, 9, 21, 0, 12, 43, 145224192, time.UTC)
	end := time.Date(2262, 04, 11, 23, 47, 16, 854775807, time.UTC)

	for _, test := range []struct {
		desc    string
		aTime   time.Time
		wantErr bool
	}{
		{desc: "ignored zero value", aTime: time.Time{}, wantErr: false},
		{desc: "earliest valid time", aTime: start, wantErr: false},
		{desc: "latested valid time", aTime: end, wantErr: false},
		{desc: "invalid times before 1678", aTime: start.Add(-1), wantErr: true},
		{desc: "invalid times after 2262", aTime: end.Add(1), wantErr: true},
		{desc: "valid times after 1678", aTime: start.Add(1), wantErr: false},
		{desc: "valid times before 2262", aTime: end.Add(-1), wantErr: false},
	} {
		tm := &TableMetadataToUpdate{ExpirationTime: test.aTime}
		_, err := tm.toBQ()
		if test.wantErr && err == nil {
			t.Errorf("[%s] got no error, want error", test.desc)
		}
		if !test.wantErr && err != nil {
			t.Errorf("[%s] got error, want no error", test.desc)
		}
	}
}

func TestTableIdentifiers(t *testing.T) {
	testTable := &Table{
		ProjectID: "p",
		DatasetID: "d",
		TableID:   "t",
		c:         nil,
	}
	for _, tc := range []struct {
		description string
		in          *Table
		format      IdentifierFormat
		want        string
		wantErr     bool
	}{
		{
			description: "empty format string",
			in:          testTable,
			format:      "",
			wantErr:     true,
		},
		{
			description: "legacy",
			in:          testTable,
			format:      LegacySQLID,
			want:        "p:d.t",
		},
		{
			description: "standard unquoted",
			in:          testTable,
			format:      StandardSQLID,
			want:        "p.d.t",
		},
		{
			description: "standard w/dash",
			in:          &Table{ProjectID: "p-p", DatasetID: "d", TableID: "t"},
			format:      StandardSQLID,
			want:        "p-p.d.t",
		},
		{
			description: "api resource",
			in:          testTable,
			format:      StorageAPIResourceID,
			want:        "projects/p/datasets/d/tables/t",
		},
	} {
		got, err := tc.in.Identifier(tc.format)
		if tc.wantErr && err == nil {
			t.Errorf("case %q: wanted err, was success", tc.description)
		}
		if !tc.wantErr {
			if err != nil {
				t.Errorf("case %q: wanted success, got err: %v", tc.description, err)
			} else {
				if got != tc.want {
					t.Errorf("case %q:  got %s, want %s", tc.description, got, tc.want)
				}
			}
		}
	}
}
