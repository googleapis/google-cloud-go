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
	"google.golang.org/api/iterator"
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

func (fs *resources) cleanResources(ctx context.Context, c *Client, resource storage_v1_tests.Resource) error {
	switch resource {
	case storage_v1_tests.Resource_OBJECT:
		// not necessary since every object must be attached to a bucket
		fs.object = nil
	case storage_v1_tests.Resource_NOTIFICATION:
		// not necessary since notifications are attached to buckets
		fs.notification = nil
	case storage_v1_tests.Resource_HMAC_KEY:
		_, doesNotExist := c.HMACKeyHandle(projectID, fs.hmacKey.AccessID).Get(ctx)
		if doesNotExist != nil {
			// assume key no longer exists, skip
			break
		}
		c.HMACKeyHandle(projectID, fs.hmacKey.AccessID).Update(ctx, HMACKeyAttrsToUpdate{State: "INACTIVE"})
		if err := c.HMACKeyHandle(projectID, fs.hmacKey.AccessID).Delete(ctx); err != nil {
			return err
		}
		fs.hmacKey = nil
	case storage_v1_tests.Resource_BUCKET:
		bucket := c.Bucket(fs.bucket.Name)
		_, doesNotExist := bucket.Attrs(ctx)
		if doesNotExist != nil {
			// assume bucket no longer exists, skip
			break
		}
		// Delete files from bucket before deleting bucket
		it := bucket.Objects(ctx, nil)
		attrs, err := it.Next()
		for err == nil {
			obj := bucket.Object(attrs.Name).If(Conditions{GenerationMatch: attrs.Generation})
			obj.Delete(ctx)
			attrs, err = it.Next()
		}
		if err != iterator.Done {
			return err
		}
		if err := bucket.Delete(ctx); err != nil {
			return err
		}
		fs.bucket = nil
	}
	return nil
}

type retryFunc func(ctx context.Context, c *Client, fs *resources, preconditions bool) error

// Methods to retry. This is a map whose keys are a string describing a standard
// API call (e.g. storage.objects.get) and values are a list of functions which
// wrap library methods that implement these calls. There may be multiple values
// because multiple library methods may use the same call (e.g. get could be a
// read or just a metadata get).
// There may be missing methods with respect to the json API as not all methods
// are used in the client library.
var methods = map[string][]retryFunc{
	"storage.bucket_acl.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).ACL().List(ctx)
			return err
		},
	},
	"storage.buckets.delete": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			// Delete files from bucket before deleting bucket
			it := c.Bucket(fs.bucket.Name).Objects(ctx, nil)
			for {
				attrs, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return err
				}
				if err := c.Bucket(fs.bucket.Name).Object(attrs.Name).Delete(ctx); err != nil {
					return err
				}
			}
			return c.Bucket(fs.bucket.Name).Delete(ctx)
		},
	},
	"storage.buckets.get": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).Attrs(ctx)
			return err
		},
	},
	"storage.buckets.getIamPolicy": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).IAM().Policy(ctx)
			return err
		},
	},
	"storage.buckets.insert": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket("bucket").Create(ctx, projectID, nil)
		},
	},
	"storage.buckets.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			it := c.Buckets(ctx, projectID)
			_, err := it.Next()
			if err == iterator.Done {
				return nil
			}
			return err
		},
	},
	"storage.buckets.lockRetentionPolicy": {
		// func(ctx context.Context, c *Client, fs *resources, _ bool) error {
		// 	//testbench currently causes issues here
		// 	return nil
		// 	bucketAttrsToUpdate := BucketAttrsToUpdate{
		// 		RetentionPolicy: &RetentionPolicy{RetentionPeriod: time.Hour * 24},
		// 	}
		// 	if _, err := c.Bucket(fs.bucket.Name).Update(ctx, bucketAttrsToUpdate); err != nil {
		// 		return err
		// 	}

		// 	attrs, err := c.Bucket(fs.bucket.Name).Attrs(ctx)
		// 	if err != nil {
		// 		return err
		// 	}
		// 	return c.Bucket(fs.bucket.Name).If(BucketConditions{MetagenerationMatch: attrs.MetaGeneration}).LockRetentionPolicy(ctx)
		// },
	},
	"storage.buckets.testIamPermissions": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).IAM().TestPermissions(ctx, nil)
			return err
		},
	},
	"storage.default_object_acl.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).DefaultObjectACL().List(ctx)
			return err
		},
	},
	"storage.hmacKey.delete": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			// key must be inactive to delete:
			c.HMACKeyHandle(projectID, fs.hmacKey.AccessID).Update(ctx, HMACKeyAttrsToUpdate{State: "INACTIVE"})
			return c.HMACKeyHandle(projectID, fs.hmacKey.AccessID).Delete(ctx)
		},
	},
	"storage.hmacKey.get": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.HMACKeyHandle(projectID, fs.hmacKey.AccessID).Get(ctx)
			return err
		},
	},
	"storage.hmacKey.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			it := c.ListHMACKeys(ctx, projectID)
			_, err := it.Next()
			return err
		},
	},
	"storage.notifications.delete": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).DeleteNotification(ctx, fs.notification.ID)
		},
	},
	"storage.notifications.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).Notifications(ctx)
			return err
		},
	},
	"storage.object_acl.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.Bucket(fs.bucket.Name).Object(fs.object.Name).ACL().List(ctx)
			return err
		},
	},
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
	"storage.objects.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			it := c.Bucket(fs.bucket.Name).Objects(ctx, nil)
			_, err := it.Next()
			return err
		},
	},
	"storage.serviceaccount.get": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.ServiceAccount(ctx, projectID)
			return err
		},
	},
	// all conditionally idempotent operations currently fail:
	"storage.buckets.patch": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			uattrs := BucketAttrsToUpdate{StorageClass: "ARCHIVE"}
			bkt := c.Bucket(fs.bucket.Name)
			if preconditions {
				bkt = c.Bucket(fs.bucket.Name).If(BucketConditions{MetagenerationMatch: fs.bucket.MetaGeneration})
			}
			_, err := bkt.Update(ctx, uattrs)
			return err
		},
	},
	"storage.buckets.setIamPolicy": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			bkt := c.Bucket(fs.bucket.Name)
			policy, err := bkt.IAM().Policy(ctx)
			if err != nil {
				return err
			}

			if err := bkt.IAM().SetPolicy(ctx, policy); err != nil {
				return err
			}
			return fmt.Errorf("Etag preconditions not supported")
		},
	},
	"storage.hmacKey.update": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			key := c.HMACKeyHandle(projectID, fs.hmacKey.AccessID)

			_, err := key.Update(ctx, HMACKeyAttrsToUpdate{State: "INACTIVE"})
			if err != nil {
				return err
			}
			return fmt.Errorf("Etag preconditions not supported")
		},
	},
	"storage.objects.compose": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			dstName := "new-object"
			src := c.Bucket(fs.bucket.Name).Object(fs.object.Name)
			dst := c.Bucket(fs.bucket.Name).Object(dstName)

			if preconditions {
				dst = c.Bucket(fs.bucket.Name).Object(dstName).If(Conditions{DoesNotExist: true})
			}

			_, err := dst.ComposerFrom(src).Run(ctx)
			//currently always retries !
			return err
		},
	},
	"storage.objects.delete": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			obj := c.Bucket(fs.bucket.Name).Object(fs.object.Name)

			if preconditions {
				obj = c.Bucket(fs.bucket.Name).Object(fs.object.Name).If(Conditions{GenerationMatch: fs.object.Generation})
			}
			//always retries
			return obj.Delete(ctx)
		},
	},
	"storage.objects.insert": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			obj := c.Bucket(fs.bucket.Name).Object("new-object.txt")

			if preconditions {
				obj = obj.If(Conditions{DoesNotExist: true})
			}

			objW := obj.NewWriter(ctx)
			if _, err := io.Copy(objW, strings.NewReader("object body")); err != nil {
				return fmt.Errorf("io.Copy: %v", err)
			}
			if err := objW.Close(); err != nil {
				return fmt.Errorf("Writer.Close: %v", err)
			}
			//always retries
			return nil
		},
	},
	"storage.objects.patch": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			uattrs := ObjectAttrsToUpdate{Metadata: map[string]string{"foo": "bar"}}
			obj := c.Bucket(fs.bucket.Name).Object(fs.object.Name)
			if preconditions {
				obj = obj.If(Conditions{MetagenerationMatch: fs.object.Metageneration})
			}
			_, err := obj.Update(ctx, uattrs)
			//always retries
			return err
		},
	},
	"storage.objects.rewrite": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			dstName := "new-object"
			src := c.Bucket(fs.bucket.Name).Object(fs.object.Name)
			dst := c.Bucket(fs.bucket.Name).Object(dstName)

			if preconditions {
				dst = c.Bucket(fs.bucket.Name).Object(dstName).If(Conditions{DoesNotExist: true})
			}

			_, err := dst.CopierFrom(src).Run(ctx)
			// always retries!!
			return err
		},
	},
	"storage.bucket_acl.delete": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).ACL().Delete(ctx, AllUsers)
		},
	},
	"storage.bucket_acl.update": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).ACL().Set(ctx, AllAuthenticatedUsers, RoleOwner)
		},
	},
	"storage.default_object_acl.delete": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).DefaultObjectACL().Delete(ctx, AllAuthenticatedUsers)
		},
	},
	"storage.default_object_acl.update": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).DefaultObjectACL().Set(ctx, AllAuthenticatedUsers, RoleOwner)
		},
	},
	"storage.hmacKey.create": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.CreateHMACKey(ctx, projectID, serviceAccountEmail)
			return err
		},
	},
	"storage.notifications.insert": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			notification := Notification{
				TopicID:        "my-topic",
				TopicProjectID: projectID,
				PayloadFormat:  "json",
			}
			_, err := c.Bucket(fs.bucket.Name).AddNotification(ctx, &notification)
			return err
		},
	},
	"storage.object_acl.delete": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).Object(fs.object.Name).ACL().Delete(ctx, AllAuthenticatedUsers)
		},
	},
	"storage.object_acl.update": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			return c.Bucket(fs.bucket.Name).Object(fs.object.Name).ACL().Set(ctx, AllAuthenticatedUsers, RoleOwner)
		},
	},
}

func TestRetryConformance(t *testing.T) {
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Skip("This test must use the testbench emulator; set STORAGE_EMULATOR_HOST to run.")
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
							subtest := &retrySubtest{T: t, name: testName}
							err := subtest.create(host, map[string][]string{
								method.Name: instructions.Instructions,
							})
							if err != nil {
								t.Fatalf("setting up retry test: %v", err)
							}

							// Create necessary test resources in the emulator
							fs := &resources{}
							for _, resource := range method.Resources {
								if err := fs.populate(ctx, client, resource); err != nil {
									t.Fatalf("creating test resources: %v", err)
								}
							}

							// Test
							err = fn(ctx, subtest.wrappedClient, fs, retryTest.PreconditionProvided)
							if retryTest.ExpectSuccess && err != nil {
								t.Errorf("want success, got %v", err)
							}
							if !retryTest.ExpectSuccess && err == nil {
								t.Errorf("want failure, got success")
							}

							// Verify that all instructions were used up during the test
							// (indicates that the client sent the correct requests).
							if err := subtest.check(); err != nil {
								t.Errorf("checking instructions: %v", err)
							}

							// Close out test in emulator.
							if err := subtest.delete(); err != nil {
								t.Errorf("deleting retry test: %v", err)
							}

							// Clean resources
							for _, resource := range method.Resources {
								if err := fs.cleanResources(ctx, client, resource); err != nil {
									t.Fatalf("cleaning test resources: %v", err)
								}
							}
						})
					}

				}
			}
		}
	}

}

type retrySubtest struct {
	*testing.T
	name          string
	id            string   // ID to pass as a header in the test execution
	host          *url.URL // set the path when using; path is not guaranteed betwen calls
	wrappedClient *Client
}

// Create a retry test resource in the emulator
func (rt *retrySubtest) create(host string, instructions map[string][]string) error {
	endpoint, err := parseURL(host)
	if err != nil {
		return err
	}
	rt.host = endpoint

	c := http.DefaultClient
	data := struct {
		Instructions map[string][]string `json:"instructions"`
	}{
		Instructions: instructions,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		return fmt.Errorf("encoding request: %v", err)
	}

	rt.host.Path = "retry_test"
	resp, err := c.Post(rt.host.String(), "application/json", buf)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("creating retry test: err: %v, resp: %+v", err, resp)
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
		return fmt.Errorf("decoding test ID: %v", err)
	}

	rt.id = testRes.TestID

	// Create wrapped client which will send emulator instructions
	rt.host.Path = ""
	client, err := wrappedClient(rt.T, rt.host.String(), rt.id)
	if err != nil {
		return fmt.Errorf("creating wrapped client: %v", err)
	}
	rt.wrappedClient = client

	return nil
}

// Verify that all instructions for a given retry testID have been used up.
func (rt *retrySubtest) check() error {
	rt.host.Path = strings.Join([]string{"retry_test", rt.id}, "/")
	c := http.DefaultClient
	resp, err := c.Get(rt.host.String())
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("getting retry test: err: %v, resp: %+v", err, resp)
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
		return fmt.Errorf("decoding response: %v", err)
	}
	if !testRes.Completed {
		return fmt.Errorf("test not completed; unused instructions: %+v", testRes.Instructions)
	}
	return nil
}

// Delete a retry test resource.
func (rt *retrySubtest) delete() error {
	rt.host.Path = strings.Join([]string{"retry_test", rt.id}, "/")
	c := http.DefaultClient
	req, err := http.NewRequest("DELETE", rt.host.String(), nil)
	if err != nil {
		return fmt.Errorf("creating request: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("deleting test: err: %v, resp: %+v", err, resp)
	}

	return nil
}

type testRoundTripper struct {
	*testing.T
	rt     http.RoundTripper
	testID string
}

func (wt *testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("x-retry-test-id", wt.testID)

	// requestDump, err := httputil.DumpRequest(r, true)
	// if err != nil {
	// 	wt.Logf("error creating request dump: %v", err)
	// }

	resp, err := wt.rt.RoundTrip(r)

	// if err != nil {
	// wt.Logf("roundtrip error (may be expected): %v\nrequest: %s", err, requestDump)

	// if resp != nil {
	// 	responseDump, err := httputil.DumpResponse(resp, true)
	// 	if err != nil {
	// 		wt.Logf("error creating response dump: %v", err)
	// 	}
	// 	wt.Logf("roundtrip error (may be expected): %v\nresponse: %s", err, responseDump)
	// }
	//}
	return resp, err
}

// Create custom client that sends instruction
func wrappedClient(t *testing.T, host, testID string) (*Client, error) {
	ctx := context.Background()
	base := http.DefaultTransport
	trans, err := htransport.NewTransport(ctx, base, option.WithScopes(raw.DevstorageFullControlScope),
		option.WithUserAgent("custom-user-agent"))
	if err != nil {
		return nil, fmt.Errorf("failed to create http client: %v", err)
	}
	c := http.Client{Transport: trans}

	// Add RoundTripper to the created HTTP client
	wrappedTrans := &testRoundTripper{rt: c.Transport, testID: testID, T: t}
	c.Transport = wrappedTrans

	// Supply this client to storage.NewClient
	client, err := NewClient(ctx, option.WithHTTPClient(&c), option.WithEndpoint(host+"/storage/v1/"))
	return client, err
}

// A url is only parsed correctly by the url package if it has a scheme,
// so we have to check and build it ourselves if not supplied in host
// Assumes http if not provided
func parseURL(host string) (*url.URL, error) {
	if strings.Contains(host, "://") {
		return url.Parse(host)
	} else {
		url := &url.URL{Scheme: "http", Host: host}
		return url, nil
	}
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
