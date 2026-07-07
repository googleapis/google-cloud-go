// Copyright 2026 Google LLC
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

package internal

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MetricsProvider is a wrapper for the built-in metrics meter
// provider. Callers pass an implementation to ClientConfig; today the
// only concrete implementations are NoopMetricsProvider (opt out of
// built-in metrics) and nil (opt in). Re-exported from the bigtable
// package as bigtable.MetricsProvider / bigtable.NoopMetricsProvider
// via type alias.
type MetricsProvider interface {
	isMetricsProvider()
}

// NoopMetricsProvider disables the built-in metrics.
type NoopMetricsProvider struct{}

// isMetricsProvider marks NoopMetricsProvider as a MetricsProvider.
func (NoopMetricsProvider) isMetricsProvider() {}

// methodNameReadRows is the method label used on operation-latencies /
// first-response-latencies when the caller is ReadRows. Duplicated
// from bigtable.methodNameReadRows so the tracer can name-match
// without importing bigtable.
const methodNameReadRows = "ReadRows"

// convertToGrpcStatusErr mirrors bigtable.convertToGrpcStatusErr —
// tracer paths need the (code, err) shape at attempt/operation
// completion, with the error canonicalized to a plain status.Error so
// downstream logging doesn't leak wrapping/details. Code extraction is
// shared with the code-only callers via GrpcCodeOf.
func convertToGrpcStatusErr(err error) (codes.Code, error) {
	code := GrpcCodeOf(err)
	if err == nil {
		return code, nil
	}
	if s, ok := status.FromError(err); ok {
		return code, status.Error(code, s.Message())
	}
	if code != codes.Unknown {
		// Context error path — canonicalize with the ctx-derived message.
		return code, status.Error(code, status.FromContextError(err).Message())
	}
	return code, err
}
