// Copyright 2018 Google LLC
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
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCopyMissingFields(t *testing.T) {
	// Verify that copying checks for missing fields.a
	t.Parallel()
	var tests = []struct {
		srcBucket, srcName, destBucket, destName string
		errMsg                                   string
	}{
		{
			"mybucket", "", "mybucket", "destname",
			"name is empty",
		},
		{
			"mybucket", "srcname", "mybucket", "",
			"name is empty",
		},
		{
			"", "srcfile", "mybucket", "destname",
			"name is empty",
		},
		{
			"mybucket", "srcfile", "", "destname",
			"name is empty",
		},
	}
	ctx := context.Background()
	client := mockClient(t, &mockTransport{})
	for i, test := range tests {
		src := client.Bucket(test.srcBucket).Object(test.srcName)
		dst := client.Bucket(test.destBucket).Object(test.destName)
		_, err := dst.CopierFrom(src).Run(ctx)
		if !strings.Contains(err.Error(), test.errMsg) {
			t.Errorf("CopyTo test #%v:\ngot err  %q\nwant err %q", i, err, test.errMsg)
		}
	}
}

func TestCopyBothEncryptionKeys(t *testing.T) {
	// Test that using both a customer-supplied key and a KMS key is an error.
	ctx := context.Background()
	client := mockClient(t, &mockTransport{})
	dest := client.Bucket("b").Object("d").Key(testEncryptionKey)
	c := dest.CopierFrom(client.Bucket("b").Object("s"))
	c.DestinationKMSKeyName = "key"
	if _, err := c.Run(ctx); err == nil {
		t.Error("got nil, want error")
	} else if !strings.Contains(err.Error(), "KMS") {
		t.Errorf(`got %q, want it to contain "KMS"`, err)
	}
}

// This test checks that request tokens are properly populated for Copy
func TestCopyTokens(t *testing.T) {
	ctx := context.Background()

	expectedTokens := make(chan string, 10)
	sendTokens := make(chan string, 10)

	client := Client{
		tc: mockRewriteObject{
			expectedTokens: expectedTokens,
			sendTokens:     sendTokens,
		},
	}

	expectedTokens <- "" // first token should always be empty
	sendTokens <- "atoken"
	expectedTokens <- "atoken"
	sendTokens <- "last123tokenexpected"
	expectedTokens <- "last123tokenexpected"
	sendTokens <- "sendToReqThatFinishes"
	sendTokens <- "lastTokenToResume"

	close(expectedTokens)
	close(sendTokens)

	dest := client.Bucket("b").Object("d").Key(testEncryptionKey)
	c := dest.CopierFrom(client.Bucket("b").Object("s"))

	if _, err := c.Run(ctx); err != nil {
		t.Fatalf("err %v", err)
	}

	if got, want := c.RewriteToken, "lastTokenToResume"; got != want {
		t.Errorf("mismatched token from copier; got: %s, want: %s", got, want)
	}
}

type mockRewriteObject struct {
	storageClient
	expectedTokens chan string
	sendTokens     chan string
}

func (m mockRewriteObject) RewriteObject(ctx context.Context, req *rewriteObjectRequest, opts ...storageOption) (*rewriteObjectResponse, error) {
	expected, ok := <-m.expectedTokens
	send := <-m.sendTokens
	if !ok {
		return &rewriteObjectResponse{
			done:     true,
			written:  1,
			size:     1,
			token:    send,
			resource: nil,
		}, nil
	}

	if req.token != expected {
		return nil, fmt.Errorf("incorrect token from request; got: %s want: %s", req.token, expected)
	}

	return &rewriteObjectResponse{
		done:     false,
		written:  1,
		size:     1,
		token:    send,
		resource: nil,
	}, nil
}
