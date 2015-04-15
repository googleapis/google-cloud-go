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

type loadDestination interface {
	customizeLoadDst(conf *bq.JobConfigurationLoad)
}

type loadSource interface {
	customizeLoadSrc(conf *bq.JobConfigurationLoad)
}

type loadOption interface {
	customizeLoad(conf *bq.JobConfigurationLoad)
}

// MaxBadRecords returns an Option that sets the maximum number of bad records that will be ignored in data being loaded into a table.
// If this maximum is exceeded, the operation will be unsuccessful.
func MaxBadRecords(n int64) Option { return maxBadRecords(n) }

type maxBadRecords int64

func (opt maxBadRecords) customizeLoad(conf *bq.JobConfigurationLoad) {
	conf.MaxBadRecords = int64(opt)
}
func (opt maxBadRecords) implementsOption() {
}

func load(c jobInserter, dst Destination, src Source, options ...Option) (*Job, error) {
	payload := &bq.JobConfigurationLoad{}

	d := dst.(loadDestination)
	s := src.(loadSource)

	d.customizeLoadDst(payload)
	s.customizeLoadSrc(payload)

	for _, opt := range options {
		o, ok := opt.(loadOption)
		if !ok {
			return nil, fmt.Errorf("Option not applicable to dst/src pair: %#v", opt)
		}
		o.customizeLoad(payload)
	}

	return c.insertJob(&bq.Job{
		Configuration: &bq.JobConfiguration{
			Load: payload,
		},
	})
}
