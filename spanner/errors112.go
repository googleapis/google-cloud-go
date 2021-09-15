// Copyright 2019 Google LLC
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

// TODO: Remove entire file when support for Go1.12 and lower has been dropped.
//go:build !go1.13
// +build !go1.13

package spanner

import "golang.org/x/xerrors"

// Unwrap is a generic implementation of (errors|xerrors).Unwrap(error). This
// implementation uses xerrors and is included in Go 1.12 and earlier builds.
func Unwrap(err error) error {
	return xerrors.Unwrap(err)
}

// ErrorAs is a generic implementation of
// (errors|xerrors).As(error, interface{}). This implementation uses xerrors
// and is included in Go 1.12 and earlier builds.
func ErrorAs(err error, target interface{}) bool {
	return xerrors.As(err, target)
}

// ErrorIs is a generic implementation of
// (errors|xerrors).Is(error, interface{}). This implementation uses xerrors
// and is included in Go 1.12 and earlier builds.
func ErrorIs(err error, target error) bool {
	return xerrors.Is(err, target)
}
