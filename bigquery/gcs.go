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
	uri             string
	SkipLeadingRows int64
}

func (gcs *GCSReference) implementsSource() {
}

// NewGCSReference constructs a reference to a Google Cloud Storage object.
func (c *Client) NewGCSReference(bucket, name string) *GCSReference {
	return &GCSReference{
		uri: fmt.Sprintf("gs://%s/%s", bucket, name),
	}
}

func (gcs *GCSReference) customizeLoadSrc(conf *bq.JobConfigurationLoad) {
	conf.SourceUris = []string{gcs.uri}
	conf.SkipLeadingRows = gcs.SkipLeadingRows
}
