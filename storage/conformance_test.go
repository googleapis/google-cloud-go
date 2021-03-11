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

package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	storage_v1_tests "cloud.google.com/go/storage/internal/test/conformance"
	"github.com/golang/protobuf/jsonpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	raw "google.golang.org/api/storage/v1"
	htransport "google.golang.org/api/transport/http"
)

type retryFunc func(ctx context.Context, c *Client, r retryCase) error

type retryCase struct {
	description string
	// Instruction header to send to the emulator.
	instructions []string
	// Functions that will wrap the library method to call.
	// storage.objects.patch
	methods []string
	// Preconditions
	// storage.objects.patch requires this value
	preconditionsProvided bool
	// Number of times to send instructions
	forcedRetries int
	// If true, the call should succeed after a retry.
	expectSuccess bool
}

func storageGet(ctx context.Context, c *Client, r retryCase) error {
	o := c.Bucket(bucketName).Object(objName)
	if r.preconditionsProvided {
		// How do make requests without messing with forced retries?
		//
		_, err := o.If(c.Conditions{GenerationMatch: 0}).Attrs(ctx)
		return err
	} else {
		_, err := o.Object(objName).Attrs(ctx)
		return err
	}
}
var funcMap map[string]retryFunc



func TestRetryConformance(t *testing.T) {
	funcMap = map[string]retryFunc{
		"storage.objects.get": storageGet,
	}

	if os.Getenv("STORAGE_EMULATOR_HOST") == "" {
		t.Skip("This test must use the testbench emulator; set STORAGE_EMULATOR_HOST to run.")
	}

	bucketName := "cjcotter-devrel-test"
	objName := "file.txt"

	rCases := []retryCase{
		{
			description:           "Retry Idempotent operations",
			instructions:          []string{"return-503", "return-504"},
			methods:               []string{"storage.objects.get"},
			forcedRetries:         4,
			preconditionsProvided: false,
			expectSuccess:         true,
		},
		{
			description:           "Retry Non-Idempotent operations",
			instructions:          []string{"return-503", "return-504"},
			methods:               []string{"storage.objects.patch"},
			forcedRetries:         4,
			preconditionsProvided: true,
			expectSuccess:         true,
		},
		{
			description:           "Retry Non-Idempotent operations",
			instructions:          []string{"return-503", "return-504"},
			methods:               []string{"storage.objects.patch"},
			forcedRetries:         1,
			preconditionsProvided: false,
			expectSuccess:         false,
		},
	}

	ctx := context.Background()

	// Create non-wrapped client to use for setup steps.
	client, err := NewClient(ctx)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}

	for _, rc := range rCases {
		for _, i := range rc.instructions {
			for _, m := range rc.methods {
				testName := fmt.Sprintf("[%v] %v", rc.description, i)
				t.Run(testName, func(t *testing.T) {
					// Setup bucket and object
					if err := client.Bucket(bucketName).Create(ctx, "myproj", &BucketAttrs{}); err != nil {
						t.Errorf("Error creating bucket: %v", err)
					}

					w := client.Bucket(bucketName).Object(objName).NewWriter(ctx)
					if _, err := w.Write([]byte("abcdef")); err != nil {
						t.Errorf("Error writing object to emulator: %v", err)
					}
					if err := w.Close(); err != nil {
						t.Errorf("Error writing object to emulator in Close: %v", err)
					}

					wrapped, err := wrappedClient(i, rc.forcedRetries)
					if err != nil {
						t.Errorf("error creating wrapped client: %v", err)
					}
					err = funcMap[m](ctx, wrapped, rc)
					if rc.expectSuccess && err != nil {
						t.Errorf("want success, got %v", err)
					}
					if !rc.expectSuccess && err == nil {
						t.Errorf("want failure, got success")
					}
				})
			}
		}
	}
}

type withInstruction struct {
	rt      http.RoundTripper
	instr   string
	retries int
}

func (wi *withInstruction) RoundTrip(r *http.Request) (*http.Response, error) {
	if wi.retries > 0 {
		r.Header.Set("x-goog-testbench-instructions", wi.instr)
		wi.retries -= 1
	}
	log.Printf("Request: %+v\nRetries: %v\n\n", r, wi.retries)
	resp, err := wi.rt.RoundTrip(r)
	//if err != nil{
	//	log.Printf("Error: %+v", err)
	//}
	return resp, err
}

// Create custom client that sends instruction
func wrappedClient(i string, maxRetries int) (*Client, error) {
	ctx := context.Background()
	base := http.DefaultTransport
	trans, err := htransport.NewTransport(ctx, base, option.WithScopes(raw.DevstorageFullControlScope),
		option.WithUserAgent("custom-user-agent"))
	if err != nil {
		return nil, fmt.Errorf("failed to create http client: %v", err)
	}
	c := http.Client{Transport: trans}

	// Add RoundTripper to the created HTTP client.
	wrappedTrans := &withInstruction{rt: c.Transport, instr: i, retries: maxRetries}
	c.Transport = wrappedTrans

	// Supply this client to storage.NewClient
	client, err := NewClient(ctx, option.WithHTTPClient(&c), option.WithEndpoint("http://localhost:9000/storage/v1/"))
	return client, err
}

func TestPostPolicyV4Conformance(t *testing.T) {
	oldUTCNow := utcNow
	defer func() {
		utcNow = oldUTCNow
	}()

	googleAccessID, privateKey, testFiles := parseFiles(t)

	for _, testFile := range testFiles {
		for _, tc := range testFile.PostPolicyV4Tests {
			t.Run(tc.Description, func(t *testing.T) {
				pin := tc.PolicyInput
				utcNow = func() time.Time {
					return time.Unix(pin.GetTimestamp().GetSeconds(), 0).UTC()
				}

				var style URLStyle
				switch pin.UrlStyle {
				case storage_v1_tests.UrlStyle_PATH_STYLE:
					style = PathStyle()
				case storage_v1_tests.UrlStyle_VIRTUAL_HOSTED_STYLE:
					style = VirtualHostedStyle()
				case storage_v1_tests.UrlStyle_BUCKET_BOUND_HOSTNAME:
					style = BucketBoundHostname(pin.BucketBoundHostname)
				}

				var conditions []PostPolicyV4Condition
				// Build the various conditions.
				pinConds := pin.Conditions
				if pinConds != nil {
					if clr := pinConds.ContentLengthRange; len(clr) > 0 {
						conditions = append(conditions, ConditionContentLengthRange(uint64(clr[0]), uint64(clr[1])))
					}
					if sw := pinConds.StartsWith; len(sw) > 0 {
						conditions = append(conditions, ConditionStartsWith(sw[0], sw[1]))
					}
				}

				metadata := make(map[string]string, len(pin.Fields))
				for key, value := range pin.Fields {
					if strings.HasPrefix(key, "x-goog-meta") {
						metadata[key] = value
					}
				}

				got, err := GenerateSignedPostPolicyV4(pin.Bucket, pin.Object, &PostPolicyV4Options{
					GoogleAccessID: googleAccessID,
					PrivateKey:     []byte(privateKey),
					Expires:        utcNow().Add(time.Duration(pin.Expiration) * time.Second),
					Style:          style,
					Insecure:       pin.Scheme == "http",
					Conditions:     conditions,
					Fields: &PolicyV4Fields{
						ACL:                    pin.Fields["acl"],
						CacheControl:           pin.Fields["cache-control"],
						ContentEncoding:        pin.Fields["content-encoding"],
						ContentDisposition:     pin.Fields["content-disposition"],
						ContentType:            pin.Fields["content-type"],
						Metadata:               metadata,
						RedirectToURLOnSuccess: strings.TrimSpace(pin.Fields["success_action_redirect"]),
						StatusCodeOnSuccess:    mustInt(t, pin.Fields["success_action_status"]),
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				want := tc.PolicyOutput

				switch wantURL, err := url.Parse(want.Url); {
				case err != nil:
					t.Errorf("Failed to parse want.Url: %v", err)

				default:
					// Sort the headers.
					wantURL.RawQuery = wantURL.Query().Encode()
					if diff := cmp.Diff(got.URL, wantURL.String()); diff != "" {
						t.Errorf("URL mismatch: got - want +\n%s", diff)
					}
				}

				gotPolicy := b64Decode(t, got.Fields["policy"], "got")
				wantPolicy := want.ExpectedDecodedPolicy
				if diff := cmp.Diff(gotPolicy, wantPolicy); diff != "" {
					t.Fatalf("Policy mismatch: got - want +\n%s", diff)
				}
			})
		}
	}
}

func b64Decode(t *testing.T, b64Str, name string) string {
	dec, err := base64.StdEncoding.DecodeString(b64Str)
	if err != nil {
		t.Fatalf("%q: Base64 decoding failed: %v", name, err)
	}
	return string(dec)
}

func mustInt(t *testing.T, s string) int {
	if s == "" {
		return 0
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return int(i)
}

func TestSigningV4Conformance(t *testing.T) {
	oldUTCNow := utcNow
	defer func() {
		utcNow = oldUTCNow
	}()

	googleAccessID, privateKey, testFiles := parseFiles(t)

	for _, testFile := range testFiles {
		for _, tc := range testFile.SigningV4Tests {
			t.Run(tc.Description, func(t *testing.T) {
				utcNow = func() time.Time {
					return time.Unix(tc.Timestamp.Seconds, 0).UTC()
				}

				qp := url.Values{}
				if tc.QueryParameters != nil {
					for k, v := range tc.QueryParameters {
						qp.Add(k, v)
					}
				}

				var style URLStyle
				switch tc.UrlStyle {
				case storage_v1_tests.UrlStyle_PATH_STYLE:
					style = PathStyle()
				case storage_v1_tests.UrlStyle_VIRTUAL_HOSTED_STYLE:
					style = VirtualHostedStyle()
				case storage_v1_tests.UrlStyle_BUCKET_BOUND_HOSTNAME:
					style = BucketBoundHostname(tc.BucketBoundHostname)
				}

				gotURL, err := SignedURL(tc.Bucket, tc.Object, &SignedURLOptions{
					GoogleAccessID:  googleAccessID,
					PrivateKey:      []byte(privateKey),
					Method:          tc.Method,
					Expires:         utcNow().Add(time.Duration(tc.Expiration) * time.Second),
					Scheme:          SigningSchemeV4,
					Headers:         headersAsSlice(tc.Headers),
					QueryParameters: qp,
					Style:           style,
					Insecure:        tc.Scheme == "http",
				})
				if err != nil {
					t.Fatal(err)
				}
				wantURL, err := url.Parse(tc.ExpectedUrl)
				if err != nil {
					t.Fatal(err)
				}
				// Sort the headers.
				wantURL.RawQuery = wantURL.Query().Encode()

				if gotURL != wantURL.String() {
					t.Fatalf("\nwant:\t%s\ngot:\t%s", wantURL.String(), gotURL)
				}
			})
		}
	}
}

func headersAsSlice(m map[string]string) []string {
	var s []string
	for k, v := range m {
		s = append(s, fmt.Sprintf("%s:%s", k, v))
	}
	return s
}

func parseFiles(t *testing.T) (googleAccessID, privateKey string, testFiles []*storage_v1_tests.TestFile) {
	dir := "internal/test/conformance"

	inBytes, err := ioutil.ReadFile(dir + "/service-account")
	if err != nil {
		t.Fatal(err)
	}
	serviceAccount := map[string]string{}
	if err := json.Unmarshal(inBytes, &serviceAccount); err != nil {
		t.Fatal(err)
	}
	googleAccessID = serviceAccount["client_email"]
	privateKey = serviceAccount["private_key"]

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		inBytes, err := ioutil.ReadFile(dir + "/" + f.Name())
		if err != nil {
			t.Fatalf("%s: %v", f.Name(), err)
		}

		testFile := new(storage_v1_tests.TestFile)
		if err := jsonpb.Unmarshal(bytes.NewReader(inBytes), testFile); err != nil {
			t.Fatalf("unmarshalling %s: %v", f.Name(), err)
		}
		testFiles = append(testFiles, testFile)
	}
	return
}
