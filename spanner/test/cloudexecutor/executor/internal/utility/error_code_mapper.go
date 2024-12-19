// Copyright 2023 Google LLC
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

package utility

import (
	"fmt"
	"log"
	"strings"

	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrToStatus maps cloud error to Status
func ErrToStatus(e error) *spb.Status {
	log.Print(e.Error())
	if strings.Contains(e.Error(), "Transaction outcome unknown") {
		return &spb.Status{Code: int32(codes.DeadlineExceeded), Message: e.Error()}
	}
	if status.Code(e) == codes.InvalidArgument {
		return &spb.Status{Code: int32(codes.InvalidArgument), Message: e.Error()}
	}
	if status.Code(e) == codes.PermissionDenied {
		return &spb.Status{Code: int32(codes.PermissionDenied), Message: e.Error()}
	}
	if status.Code(e) == codes.Aborted {
		return &spb.Status{Code: int32(codes.Aborted), Message: e.Error()}
	}
	if status.Code(e) == codes.AlreadyExists {
		return &spb.Status{Code: int32(codes.AlreadyExists), Message: e.Error()}
	}
	if status.Code(e) == codes.Canceled {
		return &spb.Status{Code: int32(codes.Canceled), Message: e.Error()}
	}
	if status.Code(e) == codes.Internal {
		return &spb.Status{Code: int32(codes.Internal), Message: e.Error()}
	}
	if status.Code(e) == codes.FailedPrecondition {
		return &spb.Status{Code: int32(codes.FailedPrecondition), Message: e.Error()}
	}
	if status.Code(e) == codes.NotFound {
		return &spb.Status{Code: int32(codes.NotFound), Message: e.Error()}
	}
	if status.Code(e) == codes.DeadlineExceeded {
		return &spb.Status{Code: int32(codes.DeadlineExceeded), Message: e.Error()}
	}
	if status.Code(e) == codes.ResourceExhausted {
		return &spb.Status{Code: int32(codes.ResourceExhausted), Message: e.Error()}
	}
	if status.Code(e) == codes.OutOfRange {
		return &spb.Status{Code: int32(codes.OutOfRange), Message: e.Error()}
	}
	if status.Code(e) == codes.Unauthenticated {
		return &spb.Status{Code: int32(codes.Unauthenticated), Message: e.Error()}
	}
	if status.Code(e) == codes.Unimplemented {
		return &spb.Status{Code: int32(codes.Unimplemented), Message: e.Error()}
	}
	if status.Code(e) == codes.Unavailable {
		return &spb.Status{Code: int32(codes.Unavailable), Message: e.Error()}
	}
	if status.Code(e) == codes.Unknown {
		return &spb.Status{Code: int32(codes.Unknown), Message: e.Error()}
	}
	return &spb.Status{Code: int32(codes.Unknown), Message: fmt.Sprintf("Error: %v, Unsupported Spanner error code: %v", e.Error(), status.Code(e))}
}
