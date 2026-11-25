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
	"encoding/base64"
	"reflect"
	"strings"
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

func TestConditionContentLengthRange_ZeroRangeIncluded(t *testing.T) {
	t.Parallel()

	policy, err := GenerateSignedPostPolicyV4("my-bucket", "my-object", &PostPolicyV4Options{
		GoogleAccessID: "xxx@clientid",
		SignBytes:      func(b []byte) ([]byte, error) { return []byte("sig"), nil },
		Expires:        time.Now().Add(time.Hour),
		Conditions:     []PostPolicyV4Condition{ConditionContentLengthRange(0, 0)},
	})
	if err != nil {
		t.Fatalf("GenerateSignedPostPolicyV4: %v", err)
	}
	rawPolicy, err := base64.StdEncoding.DecodeString(policy.Fields["policy"])
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if !strings.Contains(string(rawPolicy), `["content-length-range",0,0]`) {
		t.Errorf("policy = %s, want condition [\"content-length-range\",0,0]", string(rawPolicy))
	}
}

func TestGenerateSignedPostPolicyV4_RejectsSlashInBucket(t *testing.T) {
	t.Parallel()

	_, err := GenerateSignedPostPolicyV4("other-bucket/prefix", "my-object", &PostPolicyV4Options{
		GoogleAccessID: "xxx@clientid",
		SignBytes:      func(b []byte) ([]byte, error) { return []byte("sig"), nil },
		Expires:        time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("GenerateSignedPostPolicyV4() = nil, want error for bucket containing '/'")
	}
	if !strings.Contains(err.Error(), "must not contain '/'") {
		t.Errorf("GenerateSignedPostPolicyV4() err = %q, want substring %q", err.Error(), "must not contain '/'")
	}
}
