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

import "golang.org/x/net/context"

func (conf *readTableConf) fetch(ctx context.Context, s service, token string) (*readDataResult, error) {
	return s.readTabledata(ctx, conf, token)
}

func (conf *readTableConf) setPaging(pc *pagingConf) { conf.paging = *pc }

// Read fetches the contents of the table.
func (t *Table) Read(ctx context.Context) *RowIterator {
	conf := &readTableConf{}
	t.customizeReadSrc(conf)
	return newRowIterator(ctx, t.c.service, conf)
}

func (conf *readQueryConf) fetch(ctx context.Context, s service, token string) (*readDataResult, error) {
	return s.readQuery(ctx, conf, token)
}

func (conf *readQueryConf) setPaging(pc *pagingConf) { conf.paging = *pc }

// Read fetches the results of a query job.
// If j is not a query job, Read returns an error.
func (j *Job) Read(ctx context.Context) (*RowIterator, error) {
	conf := &readQueryConf{}
	if err := j.customizeReadQuery(conf); err != nil {
		return nil, err
	}
	return newRowIterator(ctx, j.service, conf), nil
}
