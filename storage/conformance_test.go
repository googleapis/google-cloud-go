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
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/uid"
	storage_v1_tests "cloud.google.com/go/storage/internal/test/conformance"
	"github.com/golang/protobuf/jsonpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	raw "google.golang.org/api/storage/v1"
	htransport "google.golang.org/api/transport/http"
)

var (
	bucketIDs           = uid.NewSpace("bucket", nil)
	objectIDs           = uid.NewSpace("object", nil)
	notificationIDs     = uid.NewSpace("notification", nil)
	projectID           = "my-project-id"
	serviceAccountEmail = "my-sevice-account@my-project-id.iam.gserviceaccount.com"
)

// Holds the resources for a particular test case. Only the necessary fields will
// be populated; others will be nil.
type resources struct {
	bucket       *BucketAttrs
	object       *ObjectAttrs
	notification *Notification
	hmacKey      *HMACKey
}

func (fs *resources) populate(ctx context.Context, c *Client, resource storage_v1_tests.Resource) error {
	switch resource {
	case storage_v1_tests.Resource_BUCKET:
		bkt := c.Bucket(bucketIDs.New())
		if err := bkt.Create(ctx, projectID, &BucketAttrs{}); err != nil {
			return fmt.Errorf("creating bucket: %v", err)
		}
		attrs, err := bkt.Attrs(ctx)
		if err != nil {
			return fmt.Errorf("getting bucket attrs: %v", err)
		}
		fs.bucket = attrs
	case storage_v1_tests.Resource_OBJECT:
		// Assumes bucket has been populated first.
		obj := c.Bucket(fs.bucket.Name).Object(objectIDs.New())
		w := obj.NewWriter(ctx)
		if _, err := w.Write([]byte("abcdef")); err != nil {
			return fmt.Errorf("writing object: %v", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("closing object: %v", err)
		}
		attrs, err := obj.Attrs(ctx)
		if err != nil {
			return fmt.Errorf("getting object attrs: %v", err)
		}
		fs.object = attrs
	case storage_v1_tests.Resource_NOTIFICATION:
		// Assumes bucket has been populated first.
		n, err := c.Bucket(fs.bucket.Name).AddNotification(ctx, &Notification{
			TopicProjectID: projectID,
			TopicID:        notificationIDs.New(),
			PayloadFormat:  JSONPayload,
		})
		if err != nil {
			return fmt.Errorf("adding notification: %v", err)
		}
		fs.notification = n
	case storage_v1_tests.Resource_HMAC_KEY:
		key, err := c.CreateHMACKey(ctx, projectID, serviceAccountEmail)
		if err != nil {
			return fmt.Errorf("creating HMAC key: %v", err)
		}
		fs.hmacKey = key
	}
	return nil
}

type retryFunc func(ctx context.Context, c *Client, fs *resources, preconditions bool) error

// Methods to retry. This is a map whose keys are a string describing a standard
// API call (e.g. storage.objects.get) and values are a list of functions which
// wrap library methods that implement these calls. There may be multiple values
// because multiple library methods may use the same call (e.g. get could be a
// read or just a metadata get).
var methods = map[string][]retryFunc{
	"storage.objects.get": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).Object(fs.object.Name).Attrs(ctx)
			return err
		},
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			r, err := c.Bucket(fs.bucket.Name).Object(fs.object.Name).NewReader(ctx)
			if err != nil {
				return err
			}
			_, err = io.Copy(ioutil.Discard, r)
			return err
		},
	},
	"storage.objects.update": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			uattrs := ObjectAttrsToUpdate{Metadata: map[string]string{"foo": "bar"}}
			obj := c.Bucket(fs.bucket.Name).Object(fs.object.Name)
			if preconditions {
				obj = obj.If(Conditions{MetagenerationMatch: fs.object.Metageneration})
			}
			_, err := obj.Update(ctx, uattrs)
			return err
		},
	},
	"storage.buckets.update": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			uattrs := BucketAttrsToUpdate{StorageClass: "ARCHIVE"}
			bkt := c.Bucket(fs.bucket.Name)
			if preconditions {
				bkt = bkt.If(BucketConditions{MetagenerationMatch: fs.bucket.MetaGeneration})
			}
			_, err := bkt.Update(ctx, uattrs)
			return err
		}},
}

func TestRetryConformance(t *testing.T) {
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Skip("This test must use the testbench emulator; set STORAGE_EMULATOR_HOST to run.")
	}
	host = "http://" + host

	ctx := context.Background()

	// Create non-wrapped client to use for setup steps.
	client, err := NewClient(ctx, option.WithEndpoint(host + "/storage/v1/"))
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}

	_, _, testFiles := parseFiles(t)
	for _, testFile := range testFiles {
		for _, tc := range testFile.RetryTests {
			for _, instructions := range tc.Cases {
				for _, m := range tc.Methods {
					if len(methods[m.Name]) == 0 {
						t.Logf("No tests for operation %v", m.Name)
					}
					for i, f := range methods[m.Name] {
						testName := fmt.Sprintf("%v-%v-%v-%v", tc.Id, instructions.Instructions, m.Name, i)
						t.Run(testName, func(t *testing.T) {

							// Create the retry test in the emulator to handle instructions.
							testID, err := createRetryTest(host, map[string][]string{
								m.Name: instructions.Instructions,
							})
							if err != nil {
								t.Fatalf("setting up retry test: %v", err)
							}

							fs := &resources{}
							for _, f := range m.Resources {
								if err := fs.populate(ctx, client, f); err != nil {
									t.Fatalf("creating test resources: %v", err)
								}
							}

							// Create wrapped client which will send emulator instructions.
							wrapped, err := wrappedClient(host, testID)
							if err != nil {
								t.Errorf("error creating wrapped client: %v", err)
							}
							err = f(ctx, wrapped, fs, tc.PreconditionProvided)
							if tc.ExpectSuccess && err != nil {
								t.Errorf("want success, got %v", err)
							}
							if !tc.ExpectSuccess && err == nil {
								t.Errorf("want failure, got success")
							}

							// Verify that all instructions were used up during the test
							// (indicates that the client sent the correct requests).
							if err := checkRetryTest(host, testID); err != nil {
								t.Errorf("checking instructions: %v", err)
							}

							// Close out test in emulator.
							if err := deleteRetryTest(host, testID); err != nil {
								t.Errorf("deleting retry test: %v", err)
							}

						})
					}

				}
			}
		}
	}

}

// Create a retry test resource in the emulator. Returns the ID to pass as
// a header in the test execution.
func createRetryTest(host string, instructions map[string][]string) (string, error) {
	endpoint := host + "/retry_test"
	c := http.DefaultClient
	data := struct {
		Instructions map[string][]string `json:"test_instructions"`
	}{
		Instructions: instructions,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		return "", fmt.Errorf("encoding request: %v", err)
	}
	resp, err := c.Post(endpoint, "application/json", buf)
	if err != nil || resp.StatusCode != 200 {
		return "", fmt.Errorf("creating retry test: err: %v, resp: %+v", err, resp)
	}
	defer resp.Body.Close()
	testRes := struct {
		TestID string `json:"id"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&testRes); err != nil {
		return "", fmt.Errorf("decoding test ID: %v", err)
	}
	return testRes.TestID, nil
}

// Verify that all instructions for a given retry testID have been used up.
func checkRetryTest(host, testID string) error {
	endpoint := strings.Join([]string{host, "retry_test", testID}, "/")
	c := http.DefaultClient
	resp, err := c.Get(endpoint)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("getting retry test: err: %v, resp: %+v", err, resp)
	}
	defer resp.Body.Close()
	testRes := struct {
		Instructions map[string][]string
		Completed bool
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&testRes); err != nil {
		return fmt.Errorf("decoding response: %v", err)
	}
	if !testRes.Completed {
		return fmt.Errorf("test not completed; unused instructions: %+v", testRes.Instructions)
	}
	return nil
}

// Delete a retry test resource.
func deleteRetryTest(host, testID string) error {
	endpoint := strings.Join([]string{host, "retry_test", testID}, "/")
	c := http.DefaultClient
	req, err := http.NewRequest("DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("creating request: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("deleting test: err: %v, resp: %+v", err, resp)
	}
	return nil
}

type withTestID struct {
	rt     http.RoundTripper
	testID string
}

func (wt *withTestID) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Add("x-retry-test-id", wt.testID)
	resp, err := wt.rt.RoundTrip(r)
	//if err != nil{
	//	log.Printf("Error: %+v", err)
	//}
	return resp, err
}

// Create custom client that sends instruction
func wrappedClient(host, testID string) (*Client, error) {
	ctx := context.Background()
	base := http.DefaultTransport
	trans, err := htransport.NewTransport(ctx, base, option.WithScopes(raw.DevstorageFullControlScope),
		option.WithUserAgent("custom-user-agent"))
	if err != nil {
		return nil, fmt.Errorf("failed to create http client: %v", err)
	}
	c := http.Client{Transport: trans}

	// Add RoundTripper to the created HTTP client.
	wrappedTrans := &withTestID{rt: c.Transport, testID: testID}
	c.Transport = wrappedTrans

	// Supply this client to storage.NewClient
	client, err := NewClient(ctx, option.WithHTTPClient(&c), option.WithEndpoint(host + "/storage/v1/"))
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
