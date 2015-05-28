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

type copyDestination interface {
	customizeCopyDst(conf *bq.JobConfigurationTableCopy, projectID string)
}

type copySource interface {
	customizeCopySrc(conf *bq.JobConfigurationTableCopy, projectID string)
}

type copyOption interface {
	customizeCopy(conf *bq.JobConfigurationTableCopy, projectID string)
}

func cp(dst Destination, src Source, c client, options []Option) (*Job, error) {
	job, options := initJobProto(c.projectID(), options)
	payload := &bq.JobConfigurationTableCopy{}

	d := dst.(copyDestination)
	s := src.(copySource)

	d.customizeCopyDst(payload, c.projectID())
	s.customizeCopySrc(payload, c.projectID())

	for _, opt := range options {
		o, ok := opt.(copyOption)
		if !ok {
			return nil, fmt.Errorf("option not applicable to dst/src pair: %#v", opt)
		}
		o.customizeCopy(payload, c.projectID())
	}

	job.Configuration = &bq.JobConfiguration{
		Copy: payload,
	}
	return c.insertJob(job)
}
