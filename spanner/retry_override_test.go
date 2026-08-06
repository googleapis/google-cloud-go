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
	"testing"

	"google.golang.org/grpc/codes"
)

func TestSuppressRetryCodesOptionSupportsMoreThanTwoCodes(t *testing.T) {
	opt := newSuppressRetryCodesOption(codes.ResourceExhausted, codes.Unavailable, codes.Aborted)

	if !opt.contains(codes.ResourceExhausted) {
		t.Fatal("expected ResourceExhausted to be suppressed")
	}
	if !opt.contains(codes.Unavailable) {
		t.Fatal("expected Unavailable to be suppressed")
	}
	if !opt.contains(codes.Aborted) {
		t.Fatal("expected Aborted to be suppressed")
	}
	if opt.contains(codes.Internal) {
		t.Fatal("did not expect Internal to be suppressed")
	}
}
