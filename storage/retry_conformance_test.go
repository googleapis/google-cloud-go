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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/internal/uid"
	storage_v1_tests "cloud.google.com/go/storage/internal/test/conformance"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

var (
	// Resource vars for retry tests
	bucketIDs           = uid.NewSpace("bucket", nil)
	objectIDs           = uid.NewSpace("object", nil)
	notificationIDs     = uid.NewSpace("notification", nil)
	projectID           = "my-project-id"
	serviceAccountEmail = "my-sevice-account@my-project-id.iam.gserviceaccount.com"
	randomBytesToWrite  = []byte("abcdef")
)

type retryFunc func(ctx context.Context, c *Client, fs *resources, preconditions bool) error

// Methods to retry. This is a map whose keys are a string describing a standard
// API call (e.g. storage.objects.get) and values are a list of functions which
// wrap library methods that implement these calls. There may be multiple values
// because multiple library methods may use the same call (e.g. get could be a
// read or just a metadata get).
var methods = map[string][]retryFunc{
	"storage.buckets.get": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).Attrs(ctx)
			return err
		},
	},
}

func TestRetryConformance(t *testing.T) {
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		// This test is currently skipped in CI as the env variable is not set
		// TODO: Add test to CI
		t.Skip("This test must use the testbench emulator; set STORAGE_EMULATOR_HOST to run.")
	}
	endpoint, err := url.Parse(host)
	if err != nil {
		t.Fatalf("error parsing emulator host (make sure it includes the scheme such as http://host): %v", err)
	}

	ctx := context.Background()

	// Create non-wrapped client to use for setup steps.
	client, err := NewClient(ctx)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}

	_, _, testFiles := parseFiles(t)

	for _, testFile := range testFiles {
		for _, retryTest := range testFile.RetryTests {
			for _, instructions := range retryTest.Cases {
				for _, method := range retryTest.Methods {
					if len(methods[method.Name]) == 0 {
						t.Logf("No tests for operation %v", method.Name)
					}
					for i, fn := range methods[method.Name] {
						testName := fmt.Sprintf("%v-%v-%v-%v", retryTest.Id, instructions.Instructions, method.Name, i)
						t.Run(testName, func(t *testing.T) {

							// Create the retry subtest
							subtest := &emulatorTest{T: t, name: testName, host: endpoint}
							subtest.create(map[string][]string{
								method.Name: instructions.Instructions,
							})

							// Create necessary test resources in the emulator
							subtest.populateResources(ctx, client, method.Resources)

							// Test
							err = fn(ctx, subtest.wrappedClient, &subtest.resources, retryTest.PreconditionProvided)
							if retryTest.ExpectSuccess && err != nil {
								t.Errorf("want success, got %v", err)
							}
							if !retryTest.ExpectSuccess && err == nil {
								t.Errorf("want failure, got success")
							}

							// Verify that all instructions were used up during the test
							// (indicates that the client sent the correct requests).
							subtest.check()

							// Close out test in emulator.
							subtest.delete()
						})
					}
				}
			}
		}
	}
}

type emulatorTest struct {
	*testing.T
	name          string
	id            string // ID to pass as a header in the test execution
	resources     resources
	host          *url.URL // set the path when using; path is not guaranteed between calls
	wrappedClient *Client
}

// Holds the resources for a particular test case. Only the necessary fields will
// be populated; others will be nil.
type resources struct {
	bucket       *BucketAttrs
	object       *ObjectAttrs
	notification *Notification
	hmacKey      *HMACKey
}

// Creates given test resources with the provided client
func (et *emulatorTest) populateResources(ctx context.Context, c *Client, resources []storage_v1_tests.Resource) {
	for _, resource := range resources {
		switch resource {
		case storage_v1_tests.Resource_BUCKET:
			bkt := c.Bucket(bucketIDs.New())
			if err := bkt.Create(ctx, projectID, &BucketAttrs{}); err != nil {
				et.Fatalf("creating bucket: %v", err)
			}
			attrs, err := bkt.Attrs(ctx)
			if err != nil {
				et.Fatalf("getting bucket attrs: %v", err)
			}
			et.resources.bucket = attrs
		case storage_v1_tests.Resource_OBJECT:
			// Assumes bucket has been populated first.
			obj := c.Bucket(et.resources.bucket.Name).Object(objectIDs.New())
			w := obj.NewWriter(ctx)
			if _, err := w.Write(randomBytesToWrite); err != nil {
				et.Fatalf("writing object: %v", err)
			}
			if err := w.Close(); err != nil {
				et.Fatalf("closing object: %v", err)
			}
			attrs, err := obj.Attrs(ctx)
			if err != nil {
				et.Fatalf("getting object attrs: %v", err)
			}
			et.resources.object = attrs
		case storage_v1_tests.Resource_NOTIFICATION:
			// Assumes bucket has been populated first.
			n, err := c.Bucket(et.resources.bucket.Name).AddNotification(ctx, &Notification{
				TopicProjectID: projectID,
				TopicID:        notificationIDs.New(),
				PayloadFormat:  JSONPayload,
			})
			if err != nil {
				et.Fatalf("adding notification: %v", err)
			}
			et.resources.notification = n
		case storage_v1_tests.Resource_HMAC_KEY:
			key, err := c.CreateHMACKey(ctx, projectID, serviceAccountEmail)
			if err != nil {
				et.Fatalf("creating HMAC key: %v", err)
			}
			et.resources.hmacKey = key
		}
	}
}

// Creates a retry test resource in the emulator
func (et *emulatorTest) create(instructions map[string][]string) {
	c := http.DefaultClient
	data := struct {
		Instructions map[string][]string `json:"instructions"`
	}{
		Instructions: instructions,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		et.Fatalf("encoding request: %v", err)
	}

	et.host.Path = "retry_test"
	resp, err := c.Post(et.host.String(), "application/json", buf)
	if err != nil || resp.StatusCode != 200 {
		et.Fatalf("creating retry test: err: %v, resp: %+v", err, resp)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if err == nil {
			err = closeErr
		}
	}()
	testRes := struct {
		TestID string `json:"id"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&testRes); err != nil {
		et.Fatalf("decoding test ID: %v", err)
	}

	et.id = testRes.TestID

	// Create wrapped client which will send emulator instructions
	et.host.Path = ""
	client, err := wrappedClient(et.T, et.host.String(), et.id)
	if err != nil {
		et.Fatalf("creating wrapped client: %v", err)
	}
	et.wrappedClient = client
}

// Verifies that all instructions for a given retry testID have been used up
func (et *emulatorTest) check() {
	et.host.Path = strings.Join([]string{"retry_test", et.id}, "/")
	c := http.DefaultClient
	resp, err := c.Get(et.host.String())
	if err != nil || resp.StatusCode != 200 {
		et.Errorf("getting retry test: err: %v, resp: %+v", err, resp)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if err == nil {
			err = closeErr
		}
	}()
	testRes := struct {
		Instructions map[string][]string
		Completed    bool
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&testRes); err != nil {
		et.Errorf("decoding response: %v", err)
	}
	if !testRes.Completed {
		et.Errorf("test not completed; unused instructions: %+v", testRes.Instructions)
	}
}

// Deletes a retry test resource
func (et *emulatorTest) delete() {
	et.host.Path = strings.Join([]string{"retry_test", et.id}, "/")
	c := http.DefaultClient
	req, err := http.NewRequest("DELETE", et.host.String(), nil)
	if err != nil {
		et.Errorf("creating request: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		et.Errorf("deleting test: err: %v, resp: %+v", err, resp)
	}
}

// retryTestRoundTripper sends the retry test ID to the emulator with each request
type retryTestRoundTripper struct {
	*testing.T
	rt     http.RoundTripper
	testID string
}

func (wt *retryTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("x-retry-test-id", wt.testID)

	requestDump, err := httputil.DumpRequest(r, false)
	if err != nil {
		wt.Logf("error creating request dump: %v", err)
	}

	resp, err := wt.rt.RoundTrip(r)
	if err != nil {
		wt.Logf("roundtrip error (may be expected): %v\nrequest: %s", err, requestDump)
	}
	return resp, err
}

// Create custom client that sends instructions to the storage testbench Retry Test API
func wrappedClient(t *testing.T, host, testID string) (*Client, error) {
	ctx := context.Background()
	base := http.DefaultTransport

	trans, err := htransport.NewTransport(ctx, base,
		option.WithoutAuthentication(), option.WithUserAgent("custom-user-agent"))
	if err != nil {
		return nil, fmt.Errorf("failed to create http client: %v", err)
	}

	//trans.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	c := http.Client{Transport: trans}

	// Add RoundTripper to the created HTTP client
	wrappedTrans := &retryTestRoundTripper{rt: c.Transport, testID: testID, T: t}
	c.Transport = wrappedTrans

	// Supply this client to storage.NewClient
	client, err := NewClient(ctx, option.WithHTTPClient(&c))
	return client, err
}
