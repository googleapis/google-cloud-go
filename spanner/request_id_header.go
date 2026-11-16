// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanner

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	"cloud.google.com/go/spanner/internal/requestid"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const xSpannerRequestIDHeader = requestid.Header

// optsWithNextRequestID bundles priors with a new header "x-goog-spanner-request-id"
func (g *grpcSpannerClient) optsWithNextRequestID(priors []gax.CallOption) []gax.CallOption {
	return g.reqIDTracker.OptsWithNextRequestID(priors)
}

func (g *grpcSpannerClient) prepareRequestIDTrackers(clientID int, channelID uint64, nthRequest *atomic.Uint32) {
	g.reqIDTracker = requestid.NewTracker(clientID, channelID, nthRequest)
}

type requestID string

func (r requestID) logicalRequestKey() string {
	return requestid.RequestID(r).LogicalRequestKey()
}

type wrapRetryFn = requestid.WrapRetryFn

// augmentErrorWithRequestID introspects error converting it to an *.Error and
// attaching the subject requestID, unless it is one of the following:
// * nil
// * context.Canceled
// * io.EOF
// * iterator.Done
// of which in this case, the original error will be attached as is, since those
// are sentinel errors used to break sensitive conditions like ending iterations.
func (r requestID) augmentErrorWithRequestID(err error) error {
	if err == nil {
		return nil
	}

	switch err {
	case iterator.Done, io.EOF, context.Canceled:
		return err

	default:
		potentialCommit := errors.Is(err, context.DeadlineExceeded)
		if code := status.Code(err); code == codes.DeadlineExceeded {
			potentialCommit = true
		}
		sErr := toSpannerErrorWithCommitInfo(err, potentialCommit)
		if sErr == nil {
			return err
		}

		spErr := sErr.(*Error)
		spErr.RequestID = string(r)
		return spErr
	}
}

func augmentErrorWithRequestID(err error, reqID string) error {
	return requestID(reqID).augmentErrorWithRequestID(err)
}

func logicalRequestKeyFromCallOptions(opts []gax.CallOption) string {
	return requestid.LogicalRequestKeyFromCallOptions(opts)
}

type requestIDWrap struct {
	*requestid.Wrap
}

func (riw *requestIDWrap) withNextRetryAttempt(attempt uint32) gax.CallOption {
	return riw.Wrap.WithNextRetryAttempt(attempt)
}

type logicalRequestIDWrap struct {
	logicalKey string
}

func (riw logicalRequestIDWrap) withNextRetryAttempt(attempt uint32) gax.CallOption {
	return requestid.LogicalWrap{LogicalKey: riw.logicalKey}.WithNextRetryAttempt(attempt)
}

func (gsc *grpcSpannerClient) generateRequestIDHeaderInjector() *requestIDWrap {
	return &requestIDWrap{Wrap: gsc.reqIDTracker.NewWrap()}
}

func (gsc *grpcSpannerClient) requestIDHeaderInjector(context.Context) (*requestIDWrap, error) {
	if gsc == nil {
		return nil, spannerErrorf(codes.Internal, "request-id header provider is unavailable")
	}
	return gsc.generateRequestIDHeaderInjector(), nil
}
