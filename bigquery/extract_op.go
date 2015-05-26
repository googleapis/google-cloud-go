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

type extractDestination interface {
	customizeExtractDst(conf *bq.JobConfigurationExtract, projectID string)
}

type extractSource interface {
	customizeExtractSrc(conf *bq.JobConfigurationExtract, projectID string)
}

type extractOption interface {
	customizeExtract(conf *bq.JobConfigurationExtract, projectID string)
}

// DisableHeader returns an Option that disables the printing of a header row in exported data.
func DisableHeader() Option { return disableHeader{} }

type disableHeader struct{}

func (opt disableHeader) implementsOption() {}

func (opt disableHeader) customizeExtract(conf *bq.JobConfigurationExtract, projectID string) {
	conf.PrintHeader = false
}

func extract(job *bq.Job, dst Destination, src Source, projectID string, options ...Option) error {
	payload := &bq.JobConfigurationExtract{}

	d := dst.(extractDestination)
	s := src.(extractSource)

	d.customizeExtractDst(payload, projectID)
	s.customizeExtractSrc(payload, projectID)

	for _, opt := range options {
		o, ok := opt.(extractOption)
		if !ok {
			return fmt.Errorf("option not applicable to dst/src pair: %#v", opt)
		}
		o.customizeExtract(payload, projectID)
	}

	job.Configuration = &bq.JobConfiguration{
		Extract: payload,
	}
	return nil
}
