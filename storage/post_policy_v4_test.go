// Copyright 2021 Google LLC
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

package storage

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestPostPolicyV4OptionsClone(t *testing.T) {
	t.Parallel()

	opts := &PostPolicyV4Options{
		GoogleAccessID: "accessID",
		PrivateKey:     []byte{},
		SignBytes: func(b []byte) ([]byte, error) {
			return b, nil
		},
		SignRawBytes: func(bytes []byte) ([]byte, error) {
			return bytes, nil
		},
		Expires:             time.Now(),
		Style:               VirtualHostedStyle(),
		Insecure:            true,
		Fields:              &PolicyV4Fields{ACL: "test-acl"},
		Conditions:          []PostPolicyV4Condition{},
		shouldHashSignBytes: true,
		Hostname:            "localhost:9000",
	}

	// Check that all fields are set to a non-zero value, so we can check that
	// clone accurately clones all fields and catch newly added fields not cloned
	reflectOpts := reflect.ValueOf(*opts)
	for i := 0; i < reflectOpts.NumField(); i++ {
		zero, err := isZeroValue(reflectOpts.Field(i))
		if err != nil {
			t.Errorf("IsZero: %v", err)
		}
		if zero {
			t.Errorf("PostPolicyV4Options field %d not set", i)
		}
	}

	// Check that fields are properly cloned
	optsClone := opts.clone()

	// We need a special comparer for functions
	signBytesComp := func(a func([]byte) ([]byte, error), b func([]byte) ([]byte, error)) bool {
		return reflect.ValueOf(a) == reflect.ValueOf(b)
	}

	if diff := cmp.Diff(opts, optsClone, cmp.Comparer(signBytesComp),
		cmp.AllowUnexported(PostPolicyV4Options{})); diff != "" {
		t.Errorf("clone does not match (original: -, cloned: +):\n%s", diff)
	}
}

func TestSignedURLAndPostPolicyV4_DoNotMutateOptions(t *testing.T) {
	t.Parallel()

	t.Run("SignedURL", func(t *testing.T) {
		origHeaders := []string{"X-Goog-Meta-Foo:  bar"}
		opts := &SignedURLOptions{
			GoogleAccessID: "test@example.iam.gserviceaccount.com",
			SignBytes:      func(b []byte) ([]byte, error) { return []byte("sig"), nil },
			Method:         "get",
			Expires:        time.Now().Add(time.Hour),
			Headers:        origHeaders,
			Scheme:         SigningSchemeV4,
		}

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := SignedURL("bucket", "object", opts); err != nil {
					t.Errorf("SignedURL() failed: %v", err)
				}
			}()
		}
		wg.Wait()

		if opts.Method != "get" {
			t.Errorf("opts.Method mutated to %q, want %q", opts.Method, "get")
		}
		if opts.Style != nil {
			t.Errorf("opts.Style mutated to %v, want nil", opts.Style)
		}
		if diff := cmp.Diff([]string{"X-Goog-Meta-Foo:  bar"}, opts.Headers); diff != "" {
			t.Errorf("opts.Headers mutated (-want +got):\n%s", diff)
		}
	})

	t.Run("GenerateSignedPostPolicyV4", func(t *testing.T) {
		opts := &PostPolicyV4Options{
			GoogleAccessID: "test@example.iam.gserviceaccount.com",
			SignBytes:      func(b []byte) ([]byte, error) { return []byte("sig"), nil },
			Expires:        time.Now().Add(time.Hour),
		}

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := GenerateSignedPostPolicyV4("bucket", "object", opts); err != nil {
					t.Errorf("GenerateSignedPostPolicyV4() failed: %v", err)
				}
			}()
		}
		wg.Wait()

		if opts.Style != nil {
			t.Errorf("opts.Style mutated to %v, want nil", opts.Style)
		}
		if opts.shouldHashSignBytes {
			t.Error("opts.shouldHashSignBytes mutated to true, want false")
		}
	})
}
