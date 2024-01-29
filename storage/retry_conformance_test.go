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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/internal/uid"
	storage_v1_tests "cloud.google.com/go/storage/internal/test/conformance"
	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/api/iterator"
)

var (
	// Resource vars for retry tests
	bucketIDs           = uid.NewSpace("bucket", nil)
	objectIDs           = uid.NewSpace("object", nil)
	notificationIDs     = uid.NewSpace("notification", nil)
	projectID           = "my-project-id"
	serviceAccountEmail = "my-sevice-account@my-project-id.iam.gserviceaccount.com"
	randomBytesToWrite  = []byte("abcdef")
	randomBytes9MB      = generateRandomBytes(size9MB)
	size9MB             = 9437184 // 9 MiB
)

type retryFunc func(ctx context.Context, c *Client, fs *resources, preconditions bool) error

// Methods to retry. This is a map whose keys are a string describing a standard
// API call (e.g. storage.objects.get) and values are a list of functions which
// wrap library methods that implement these calls. There may be multiple values
// because multiple library methods may use the same call (e.g. get could be a
// read or just a metadata get).
//
// There may be missing methods with respect to the json API as not all methods
// are used in the client library. The following are not used:
// storage.bucket_acl.get
// storage.bucket_acl.insert
// storage.bucket_acl.patch
// storage.buckets.update
// storage.default_object_acl.get
// storage.default_object_acl.insert
// storage.default_object_acl.patch
// storage.notifications.get
// storage.object_acl.get
// storage.object_acl.insert
// storage.object_acl.patch
// storage.objects.copy
// storage.objects.update
var methods = map[string][]retryFunc{
	// Idempotent operations
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
			b := bucketIDs.New()
			return c.Bucket(b).Create(ctx, projectID, nil)
		},
	},
	"storage.buckets.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			it := c.Buckets(ctx, projectID)
			for {
				_, err := it.Next()
				if err == iterator.Done {
					return nil
				}
				if err != nil {
					return err
				}
			}
		},
	},
	"storage.buckets.lockRetentionPolicy": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			// buckets.lockRetentionPolicy is always idempotent, but is a special case because IfMetagenerationMatch is always required
			return c.Bucket(fs.bucket.Name).If(BucketConditions{MetagenerationMatch: fs.bucket.MetaGeneration}).LockRetentionPolicy(ctx)
		},
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
			for {
				_, err := it.Next()
				if err == iterator.Done {
					return nil
				}
				if err != nil {
					return err
				}
			}
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
			wr, err := io.Copy(ioutil.Discard, r)
			if got, want := wr, len(randomBytesToWrite); got != int64(want) {
				return fmt.Errorf("body length mismatch\ngot:\n%v\n\nwant:\n%v", got, want)
			}
			return err
		},
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			// Test JSON reads.
			client, ok := c.tc.(*httpStorageClient)
			if ok {
				client.config.readAPIWasSet = true
				client.config.useJSONforReads = true
				defer func() {
					client.config.readAPIWasSet = false
					client.config.useJSONforReads = false
				}()
			}

			r, err := c.Bucket(fs.bucket.Name).Object(fs.object.Name).NewReader(ctx)
			if err != nil {
				return err
			}
			wr, err := io.Copy(ioutil.Discard, r)
			if got, want := wr, len(randomBytesToWrite); got != int64(want) {
				return fmt.Errorf("body length mismatch\ngot:\n%v\n\nwant:\n%v", got, want)
			}
			return err
		},
	},
	"storage.objects.download": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			// Before running the test method, populate a large test object of 9 MiB.
			objName := objectIDs.New()
			if err := uploadTestObject(fs.bucket.Name, objName, randomBytes9MB); err != nil {
				return fmt.Errorf("failed to create 9 MiB large object pre test, err: %v", err)
			}
			// Download the large test object for the S8 download method group.
			r, err := c.Bucket(fs.bucket.Name).Object(objName).NewReader(ctx)
			if err != nil {
				return err
			}
			defer r.Close()
			data, err := ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("failed to ReadAll, err: %v", err)
			}
			if got, want := len(data), size9MB; got != want {
				return fmt.Errorf("body length mismatch\ngot:\n%v\n\nwant:\n%v", got, want)
			}
			if got, want := data, randomBytes9MB; !bytes.Equal(got, want) {
				return fmt.Errorf("body mismatch\ngot:\n%v\n\nwant:\n%v", got, want)
			}
			return nil
		},
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			// Test JSON reads.
			// Before running the test method, populate a large test object of 9 MiB.
			objName := objectIDs.New()
			if err := uploadTestObject(fs.bucket.Name, objName, randomBytes9MB); err != nil {
				return fmt.Errorf("failed to create 9 MiB large object pre test, err: %v", err)
			}

			client, ok := c.tc.(*httpStorageClient)
			if ok {
				client.config.readAPIWasSet = true
				client.config.useJSONforReads = true
				defer func() {
					client.config.readAPIWasSet = false
					client.config.useJSONforReads = false
				}()
			}

			// Download the large test object for the S8 download method group.
			r, err := c.Bucket(fs.bucket.Name).Object(objName).NewReader(ctx)
			if err != nil {
				return err
			}
			defer r.Close()
			data, err := ioutil.ReadAll(r)
			if err != nil {
				return fmt.Errorf("failed to ReadAll, err: %v", err)
			}
			if got, want := len(data), size9MB; got != want {
				return fmt.Errorf("body length mismatch\ngot:\n%v\n\nwant:\n%v", got, want)
			}
			if got, want := data, randomBytes9MB; !bytes.Equal(got, want) {
				return fmt.Errorf("body mismatch\ngot:\n%v\n\nwant:\n%v", got, want)
			}
			return nil
		},
	},
	"storage.objects.list": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			it := c.Bucket(fs.bucket.Name).Objects(ctx, nil)
			for {
				_, err := it.Next()
				if err == iterator.Done {
					return nil
				}
				if err != nil {
					return err
				}
			}
		},
	},
	"storage.serviceaccount.get": {
		func(ctx context.Context, c *Client, fs *resources, _ bool) error {
			_, err := c.ServiceAccount(ctx, projectID)
			return err
		},
	},
	// Conditionally idempotent operations
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

			if !preconditions {
				policy.InternalProto.Etag = nil
			}

			return bkt.IAM().SetPolicy(ctx, policy)
		},
	},
	"storage.hmacKey.update": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			key := c.HMACKeyHandle(projectID, fs.hmacKey.AccessID)
			uattrs := HMACKeyAttrsToUpdate{State: "INACTIVE"}

			if preconditions {
				uattrs.Etag = fs.hmacKey.Etag
			}

			_, err := key.Update(ctx, uattrs)
			return err
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
			return err
		},
	},
	"storage.objects.delete": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			obj := c.Bucket(fs.bucket.Name).Object(fs.object.Name)

			if preconditions {
				obj = c.Bucket(fs.bucket.Name).Object(fs.object.Name).If(Conditions{GenerationMatch: fs.object.Generation})
			}
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
			return nil
		},
	},
	"storage.resumable.upload": {
		func(ctx context.Context, c *Client, fs *resources, preconditions bool) error {
			obj := c.Bucket(fs.bucket.Name).Object(objectIDs.New())
			if preconditions {
				obj = obj.If(Conditions{DoesNotExist: true})
			}
			w := obj.NewWriter(ctx)
			// Set Writer.ChunkSize to 2 MiB to perform resumable uploads.
			w.ChunkSize = 2097152

			if _, err := w.Write(randomBytes9MB); err != nil {
				return fmt.Errorf("writing object: %v", err)
			}
			if err := w.Close(); err != nil {
				return fmt.Errorf("closing object: %v", err)
			}
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
			return err
		},
	},
	// Non-idempotent operations
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
					methodName := method.Name
					if method.Group != "" {
						methodName = method.Group
					}
					if len(methods[methodName]) == 0 {
						t.Logf("No tests for operation %v", methodName)
					}
					for i, fn := range methods[methodName] {
						transports := []string{"http", "grpc"}
						for _, transport := range transports {
							if transport == "grpc" && method.Name == "storage.objects.insert" {
								t.Skip("Implementation in testbench pending: https://github.com/googleapis/storage-testbench/issues/568")
							}
							testName := fmt.Sprintf("%v-%v-%v-%v-%v", transport, retryTest.Id, instructions.Instructions, methodName, i)
							t.Run(testName, func(t *testing.T) {

								// Create the retry subtest
								subtest := &emulatorTest{T: t, name: testName, host: endpoint}
								subtest.create(map[string][]string{
									method.Name: instructions.Instructions,
								}, transport)

								// Create necessary test resources in the emulator
								subtest.populateResources(ctx, client, method.Resources)

								// Test
								// Set retry test id through headers per test call
								ctx := context.Background()
								ctx = callctx.SetHeaders(ctx, "x-retry-test-id", subtest.id)
								err = fn(ctx, subtest.transportClient, &subtest.resources, retryTest.PreconditionProvided)
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
}

type emulatorTest struct {
	*testing.T
	name            string
	id              string // ID to pass as a header in the test execution
	resources       resources
	host            *url.URL // set the path when using; path is not guaranteed between calls
	transportClient *Client
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

// Generates size random bytes.
func generateRandomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// Upload test object with given bytes.
func uploadTestObject(bucketName, objName string, n []byte) error {
	// Create non-wrapped client to create test object.
	ctx := context.Background()
	c, err := NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	obj := c.Bucket(bucketName).Object(objName)
	w := obj.NewWriter(ctx)
	if _, err := w.Write(n); err != nil {
		return fmt.Errorf("writing test object: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing object: %v", err)
	}
	return nil
}

// Creates a retry test resource in the emulator
func (et *emulatorTest) create(instructions map[string][]string, transport string) {
	c := http.DefaultClient
	data := struct {
		Instructions map[string][]string `json:"instructions"`
		Transport    string              `json:"transport"`
	}{
		Instructions: instructions,
		Transport:    transport,
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		et.Fatalf("encoding request: %v", err)
	}

	et.host.Path = "retry_test"
	resp, err := c.Post(et.host.String(), "application/json", buf)
	if resp.StatusCode == 501 {
		et.T.Skip("This retry test case is not yet supported in the testbench.")
	}
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
	et.host.Path = ""

	// Create transportClient for http or grpc
	ctx := context.Background()
	transportClient, err := NewClient(ctx)
	if err != nil {
		et.Fatalf("HTTP transportClient: %v", err)
	}
	if transport == "grpc" {
		transportClient, err = NewGRPCClient(ctx)
		if err != nil {
			et.Fatalf("GRPC transportClient: %v", err)
		}
	}
	et.transportClient = transportClient
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
