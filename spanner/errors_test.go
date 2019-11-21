/*
Copyright 2017 Google LLC

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
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type wrappedTestError struct {
	wrapped error
	msg     string
}

func (w *wrappedTestError) Error() string {
	return w.msg
}

func (w *wrappedTestError) Unwrap() error {
	return w.wrapped
}

func TestToSpannerError(t *testing.T) {
	for _, test := range []struct {
		err      error
		wantCode codes.Code
		wantMsg  string
	}{
		{errors.New("wha?"), codes.Unknown, `spanner: code = "Unknown", desc = "wha?"`},
		{context.Canceled, codes.Canceled, `spanner: code = "Canceled", desc = "context canceled"`},
		{context.DeadlineExceeded, codes.DeadlineExceeded, `spanner: code = "DeadlineExceeded", desc = "context deadline exceeded"`},
		{status.Errorf(codes.ResourceExhausted, "so tired"), codes.ResourceExhausted, `spanner: code = "ResourceExhausted", desc = "so tired"`},
		{spannerErrorf(codes.InvalidArgument, "bad"), codes.InvalidArgument, `spanner: code = "InvalidArgument", desc = "bad"`},
		{&wrappedTestError{
			wrapped: spannerErrorf(codes.Aborted, "Transaction aborted"),
			msg:     "error with wrapped Spanner error",
		}, codes.Aborted, `spanner: code = "Aborted", desc = "Transaction aborted"`},
		{&wrappedTestError{
			wrapped: errors.New("wha?"),
			msg:     "error with wrapped non-gRPC and non-Spanner error",
		}, codes.Unknown, `spanner: code = "Unknown", desc = "error with wrapped non-gRPC and non-Spanner error"`},
	} {
		err := toSpannerError(test.err)
		if got, want := ErrCode(err), test.wantCode; got != want {
			t.Errorf("%v: got %s, want %s", test.err, got, want)
		}
		converted := status.Convert(err)
		if converted.Code() != test.wantCode {
			t.Errorf("%v: got status %v, want status %v", test.err, converted.Code(), test.wantCode)
		}
		if got, want := err.Error(), test.wantMsg; got != want {
			t.Errorf("%v: got msg %s, want mgs %s", test.err, got, want)
		}
	}
}
