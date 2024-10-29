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
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// randIDForProcess is a strongly randomly generated value derived
// from a uint64, and in the range [0, maxUint64].
var randIDForProcess string

func init() {
	bigMaxInt64, _ := new(big.Int).SetString(fmt.Sprintf("%d", uint64(math.MaxUint64)), 10)
	if g, w := bigMaxInt64.Uint64(), uint64(math.MaxUint64); g != w {
		panic(fmt.Sprintf("mismatch in randIDForProcess.maxUint64:\n\tGot:  %d\n\tWant: %d", g, w))
	}
	r64, err := rand.Int(rand.Reader, bigMaxInt64)
	if err != nil {
		panic(err)
	}
	randIDForProcess = r64.String()
}

// Please bump this version whenever this implementation
// executes on the plans of a new specification.
const xSpannerRequestIDVersion uint8 = 1

const xSpannerRequestIDHeader = "x-goog-spanner-request-id"

// optsWithNextRequestID bundles priors with a new header "x-goog-spanner-request-id"
func (g *grpcSpannerClient) optsWithNextRequestID(priors []gax.CallOption) []gax.CallOption {
	return append(priors, &retryerWithRequestID{g})
}

func (g *grpcSpannerClient) prepareRequestIDTrackers(clientID int, channelID uint64) {
	g.id = clientID // The ID derived from the SpannerClient.
	g.channelID = channelID
	g.nthRequest = new(atomic.Uint32)
}

// retryerWithRequestID is a gax.CallOption that injects "x-goog-spanner-request-id"
// into every RPC, and it appropriately increments the RPC's ordinal number per retry.
type retryerWithRequestID struct {
	gsc *grpcSpannerClient
}

var _ gax.CallOption = (*retryerWithRequestID)(nil)

func (g *grpcSpannerClient) appendRequestIDToGRPCOptions(priors []grpc.CallOption, nthRequest, nthRPC uint32) []grpc.CallOption {
	// Google Engineering has requested that each value be added in Decimal unpadded.
	// Should we have a standardized endianness: Little Endian or Big Endian?
	requestID := fmt.Sprintf("%d.%s.%d.%d.%d.%d", xSpannerRequestIDVersion, randIDForProcess, g.id, g.channelID, nthRequest, nthRPC)
	md := metadata.MD{xSpannerRequestIDHeader: []string{requestID}}
	return append(priors, grpc.Header(&md))
}

type requestID string

// augmentErrorWithRequestID introspects error converting it to an *.Error and
// attaching the subject requestID, unless it is one of the following:
// * nil
// * context.Canceled
// * context.DeadlineExceeded
// * io.EOF
// * iterator.Done
// of which in this case, the original error will be attached as it, since those
// are sentinel errors used to break sensitive conditions like ending iterations.
func (r requestID) augmentErrorWithRequestID(err error) error {
	if err == nil {
		return nil
	}

	switch code := status.Code(err); code {
	case codes.DeadlineExceeded, codes.Canceled:
		return err
	}

	switch err {
	case iterator.Done, io.EOF, context.Canceled, context.DeadlineExceeded:
		return err

	default:
		sErr := ToSpannerError(err)
		if sErr == nil {
			return err
		}

		spErr := sErr.(*Error)
		spErr.RequestID = string(r)
		return spErr
	}
}

func gRPCCallOptionsToRequestID(opts []grpc.CallOption) (reqID requestID, found bool) {
	for _, opt := range opts {
		hdrOpt, ok := opt.(grpc.HeaderCallOption)
		if !ok {
			continue
		}

		metadata := hdrOpt.HeaderAddr
		reqIDs := metadata.Get(xSpannerRequestIDHeader)
		if len(reqIDs) != 0 && len(reqIDs[0]) != 0 {
			reqID = requestID(reqIDs[0])
			found = true
			break
		}
	}
	return
}

func (wr *requestIDHeaderInjector) interceptUnary(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	// It is imperative to search for the requestID before the call
	// because gRPC's internals will consume the headers.
	reqID, foundRequestID := gRPCCallOptionsToRequestID(opts)
	err := invoker(ctx, method, req, reply, cc, opts...)
	if !foundRequestID {
		return err
	}
	return reqID.augmentErrorWithRequestID(err)
}

type requestIDErrWrappingClientStream struct {
	grpc.ClientStream
	reqID requestID
}

func (rew *requestIDErrWrappingClientStream) processFromOutgoingContext(err error) error {
	if err == nil {
		return nil
	}
	return rew.reqID.augmentErrorWithRequestID(err)
}

func (rew *requestIDErrWrappingClientStream) SendMsg(msg any) error {
	err := rew.ClientStream.SendMsg(msg)
	return rew.processFromOutgoingContext(err)
}

func (rew *requestIDErrWrappingClientStream) RecvMsg(msg any) error {
	err := rew.ClientStream.RecvMsg(msg)
	return rew.processFromOutgoingContext(err)
}

var _ grpc.ClientStream = (*requestIDErrWrappingClientStream)(nil)

type requestIDHeaderInjector int

func (wr *requestIDHeaderInjector) interceptStream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	// It is imperative to search for the requestID before the call
	// because gRPC's internals will consume the headers.
	reqID, foundRequestID := gRPCCallOptionsToRequestID(opts)
	cs, err := streamer(ctx, desc, cc, method, opts...)
	if !foundRequestID {
		return cs, err
	}
	wcs := &requestIDErrWrappingClientStream{cs, reqID}
	if err == nil {
		return wcs, nil
	}

	return wcs, reqID.augmentErrorWithRequestID(err)
}

func (wr *retryerWithRequestID) Resolve(cs *gax.CallSettings) {
	nthRequest := wr.gsc.nextNthRequest()
	nthRPC := uint32(1)
	originalGRPCOptions := cs.GRPC
	// Inject the first request-id header.
	cs.GRPC = wr.gsc.appendRequestIDToGRPCOptions(originalGRPCOptions, nthRequest, nthRPC)

	if cs.Retry == nil {
		// If there was no retry manager, our journey has ended.
		return
	}

	// Otherwise in this case for each retry, we need to increment nthRPC on every
	// retry and re-append the requestID header to the original cs.GRPC callOptions.
	originalRetryer := cs.Retry()
	newRetryer := func() gax.Retryer {
		return (wrapRetryFn)(func(err error) (pause time.Duration, shouldRetry bool) {
			nthRPC++
			cs.GRPC = wr.gsc.appendRequestIDToGRPCOptions(originalGRPCOptions, nthRequest, nthRPC)
			return originalRetryer.Retry(err)
		})
	}
	cs.Retry = newRetryer
}

type wrapRetryFn func(err error) (time.Duration, bool)

var _ gax.Retryer = (wrapRetryFn)(nil)

func (fn wrapRetryFn) Retry(err error) (time.Duration, bool) {
	return fn(err)
}

func (g *grpcSpannerClient) nextNthRequest() uint32 {
	return g.nthRequest.Add(1)
}
