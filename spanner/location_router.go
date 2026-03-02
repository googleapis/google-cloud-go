/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"os"
	"strconv"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

const experimentalLocationAPIEnvVar = "GOOGLE_SPANNER_EXPERIMENTAL_LOCATION_API"

type locationRouter struct {
	finder *channelFinder
}

func isExperimentalLocationAPIEnabled() bool {
	enabled, _ := strconv.ParseBool(os.Getenv(experimentalLocationAPIEnvVar))
	return enabled
}

func newLocationRouter() *locationRouter {
	return &locationRouter{finder: newChannelFinder(nil)}
}

func (r *locationRouter) prepareReadRequest(req *sppb.ReadRequest) {
	if r == nil || req == nil {
		return
	}
	r.finder.findServerReadWithTransaction(req)
}

func (r *locationRouter) prepareExecuteSQLRequest(req *sppb.ExecuteSqlRequest) {
	if r == nil || req == nil {
		return
	}
	r.finder.findServerExecuteSQLWithTransaction(req)
}

func (r *locationRouter) prepareBeginTransactionRequest(req *sppb.BeginTransactionRequest) {
	if r == nil || req == nil {
		return
	}
	r.finder.findServerBeginTransaction(req)
}

func (r *locationRouter) observePartialResultSet(prs *sppb.PartialResultSet) {
	if r == nil || prs == nil || prs.GetCacheUpdate() == nil {
		return
	}
	r.finder.update(prs.GetCacheUpdate())
}

func (r *locationRouter) observeResultSet(rs *sppb.ResultSet) {
	if r == nil || rs == nil || rs.GetCacheUpdate() == nil {
		return
	}
	r.finder.update(rs.GetCacheUpdate())
}
