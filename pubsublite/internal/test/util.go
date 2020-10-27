// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package test

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorEqual compares two errors for equivalence.
func ErrorEqual(got, want error) bool {
	if got == want {
		return true
	}
	return cmp.Equal(got, want, cmpopts.EquateErrors())
}

// ErrorHasCode returns true if an error has the desired canonical code.
func ErrorHasCode(got error, wantCode codes.Code) bool {
	if s, ok := status.FromError(got); ok {
		return s.Code() == wantCode
	}
	return false
}

// FakeSource is a fake source that returns a configurable constant.
type FakeSource struct {
	Ret int64
}

func (f *FakeSource) Int63() int64    { return f.Ret }
func (f *FakeSource) Seed(seed int64) {}
