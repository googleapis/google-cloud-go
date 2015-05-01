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
	"fmt"

	bq "google.golang.org/api/bigquery/v2"
)

// GCSReference is a reference to a Google Cloud Storage object.
type GCSReference struct {
	// TODO(mcgreevy): support multiple source URIs
	uri string

	// The number of rows at the top of a CSV file that BigQuery will skip when loading the data.
	SkipLeadingRows int64

	SourceFormat SourceFormat
	Encoding     Encoding

	// FieldDelimiter is the separator for fields in a CSV file, used when loading or exporting data.
	// The default is ",".
	FieldDelimiter string

	// Quote is the value used to quote data sections in a CSV file.
	// The default quotation character is the double quote ("), which is used if both Quote and ForceZeroQuote are unset.
	// To specify that no character should be interpreted as a quotation character, set ForceZeroQuote to true.
	Quote          string
	ForceZeroQuote bool
}

func (gcs *GCSReference) implementsSource() {
}

// NewGCSReference constructs a reference to a Google Cloud Storage object.
func (c *Client) NewGCSReference(bucket, name string) *GCSReference {
	return &GCSReference{
		uri: fmt.Sprintf("gs://%s/%s", bucket, name),
	}
}

// SourceFormat is the format of a data file to be loaded into BigQuery.
type SourceFormat string

const (
	CSV             SourceFormat = "CSV"
	JSON            SourceFormat = "NEWLINE_DELIMITED_JSON"
	DatastoreBackup SourceFormat = "DATASTORE_BACKUP"
)

// Encoding specifies the character encoding of data to be loaded into BigQuery.
// See https://cloud.google.com/bigquery/docs/reference/v2/jobs#configuration.load.encoding
// for more details about how this is used.
type Encoding string

const (
	UTF_8      Encoding = "UTF-8"
	ISO_8859_1 Encoding = "ISO-8859-1"
)

func (gcs *GCSReference) customizeLoadSrc(conf *bq.JobConfigurationLoad) {
	conf.SourceUris = []string{gcs.uri}
	conf.SkipLeadingRows = gcs.SkipLeadingRows
	conf.SourceFormat = string(gcs.SourceFormat)
	conf.Encoding = string(gcs.Encoding)
	conf.FieldDelimiter = gcs.FieldDelimiter

	// TODO(mcgreevy): take into account gcs.Unquoted once the underlying library supports it.
	conf.Quote = gcs.Quote
}
