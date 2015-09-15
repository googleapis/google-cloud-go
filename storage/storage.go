// Copyright 2014 Google Inc. All Rights Reserved.
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

// Package storage contains a Google Cloud Storage client.
//
// This package is experimental and may make backwards-incompatible changes.
package storage // import "google.golang.org/cloud/storage"

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/cloud"
	"google.golang.org/cloud/internal/transport"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/storage/v1"
)

var (
	ErrBucketNotExist = errors.New("storage: bucket doesn't exist")
	ErrObjectNotExist = errors.New("storage: object doesn't exist")
)

const (
	// ScopeFullControl grants permissions to manage your
	// data and permissions in Google Cloud Storage.
	ScopeFullControl = raw.DevstorageFullControlScope

	// ScopeReadOnly grants permissions to
	// view your data in Google Cloud Storage.
	ScopeReadOnly = raw.DevstorageReadOnlyScope

	// ScopeReadWrite grants permissions to manage your
	// data in Google Cloud Storage.
	ScopeReadWrite = raw.DevstorageReadWriteScope
)

// Client is a client for interacting with Google Cloud Storage.
type Client struct {
	conn *http.Client
	raw  *raw.Service

	project string
}

// NewClient creates a new Client for a given project.
func NewClient(ctx context.Context, project string, opts ...cloud.ClientOption) (*Client, error) {
	conn, _, err := transport.NewHTTPClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}
	rawService, err := raw.New(conn)
	if err != nil {
		return nil, fmt.Errorf("storage client: %v", err)
	}
	return &Client{
		conn: conn,
		raw:  rawService,

		project: project,
	}, nil
}

// Close closes the Client.
func (c *Client) Close() {
	// NOTE(cbro): no-op until we migrate the transport to gRPC.
}

// Bucket provides operations on a Google Cloud Storage bucket.
// Use Client.Bucket() to initialize the bucket.
type Bucket struct {
	// ACL provides access to the bucket's access control list.
	// This controls who can list, create or overwrite the objects in a bucket.
	ACL *ACL

	// DefaultObjectACL provides access to the bucket's default object ACLs.
	// These ACLs are applied to newly created objects in this bucket that do not
	// have a defined ACL.
	DefaultObjectACL *ACL

	c    *Client
	name string
}

// Bucket creates a Bucket for the named bucket.
func (c *Client) Bucket(name string) *Bucket {
	return &Bucket{
		c:    c,
		name: name,
		ACL: &ACL{
			c:      c,
			bucket: name,
		},
		DefaultObjectACL: &ACL{
			c:         c,
			bucket:    name,
			isDefault: true,
		},
	}
}

// Object provides operations on an object in a Google Cloud Storage bucket.
// Use Client.Bucket().Object() to initialize the object.
type Object struct {
	c      *Client
	bucket string
	object string

	ACL *ACL
}

// Object creates an Object for the named object in the bucket.
func (b *Bucket) Object(name string) *Object {
	return &Object{
		c:      b.c,
		bucket: b.name,
		object: name,
		ACL: &ACL{
			c:      b.c,
			bucket: b.name,
			object: name,
		},
	}
}

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.insert.
// TODO(jbd): Add storage.buckets.update.
// TODO(jbd): Add storage.buckets.delete.

// TODO(jbd): Add storage.objects.watch.

// Attrs returns the metadata for the bucket.
func (b *Bucket) Attrs(ctx context.Context) (*BucketAttrs, error) {
	resp, err := b.c.raw.Buckets.Get(b.name).Projection("full").Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrBucketNotExist
	}
	if err != nil {
		return nil, err
	}
	return newBucket(resp), nil
}

// List lists objects from the bucket. You can specify a query
// to filter the results. If q is nil, no filtering is applied.
func (b *Bucket) List(ctx context.Context, q *Query) (list *ObjectList, next *Query, err error) {
	req := b.c.raw.Objects.List(b.name)
	req.Projection("full")
	if q != nil {
		req.Delimiter(q.Delimiter)
		req.Prefix(q.Prefix)
		req.Versions(q.Versions)
		req.PageToken(q.Cursor)
		if q.MaxResults > 0 {
			req.MaxResults(int64(q.MaxResults))
		}
	}
	resp, err := req.Do()
	if err != nil {
		return nil, nil, err
	}
	objects := &ObjectList{
		Results:  make([]*ObjectAttrs, len(resp.Items)),
		Prefixes: make([]string, len(resp.Prefixes)),
	}
	for i, item := range resp.Items {
		objects.Results[i] = newObject(item)
	}
	for i, prefix := range resp.Prefixes {
		objects.Prefixes[i] = prefix
	}
	if resp.NextPageToken != "" {
		next := Query{}
		if q != nil {
			// keep the other filtering
			// criteria if there is a query
			next = *q
		}
		next.Cursor = resp.NextPageToken
		return objects, &next, nil
	}
	return objects, nil, nil
}

// SignedURLOptions allows you to restrict the access to the signed URL.
type SignedURLOptions struct {
	// GoogleAccessID represents the authorizer of the signed URL generation.
	// It is typically the Google service account client email address from
	// the Google Developers Console in the form of "xxx@developer.gserviceaccount.com".
	// Required.
	GoogleAccessID string

	// PrivateKey is the Google service account private key. It is obtainable
	// from the Google Developers Console.
	// At https://console.developers.google.com/project/<your-project-id>/apiui/credential,
	// create a service account client ID or reuse one of your existing service account
	// credentials. Click on the "Generate new P12 key" to generate and download
	// a new private key. Once you download the P12 file, use the following command
	// to convert it into a PEM file.
	//
	//    $ openssl pkcs12 -in key.p12 -passin pass:notasecret -out key.pem -nodes
	//
	// Provide the contents of the PEM file as a byte slice.
	// Required.
	PrivateKey []byte

	// Method is the HTTP method to be used with the signed URL.
	// Signed URLs can be used with GET, HEAD, PUT, and DELETE requests.
	// Required.
	Method string

	// Expires is the expiration time on the signed URL. It must be
	// a datetime in the future.
	// Required.
	Expires time.Time

	// ContentType is the content type header the client must provide
	// to use the generated signed URL.
	// Optional.
	ContentType string

	// Headers is a list of extention headers the client must provide
	// in order to use the generated signed URL.
	// Optional.
	Headers []string

	// MD5 is the base64 encoded MD5 checksum of the file.
	// If provided, the client should provide the exact value on the request
	// header in order to use the signed URL.
	// Optional.
	MD5 []byte
}

// SignedURL returns a URL for the specified object. Signed URLs allow
// the users access to a restricted resource for a limited time without having a
// Google account or signing in. For more information about the signed
// URLs, see https://cloud.google.com/storage/docs/accesscontrol#Signed-URLs.
func SignedURL(bucket, name string, opts *SignedURLOptions) (string, error) {
	if opts == nil {
		return "", errors.New("storage: missing required SignedURLOptions")
	}
	if opts.GoogleAccessID == "" || opts.PrivateKey == nil {
		return "", errors.New("storage: missing required credentials to generate a signed URL")
	}
	if opts.Method == "" {
		return "", errors.New("storage: missing required method option")
	}
	if opts.Expires.IsZero() {
		return "", errors.New("storage: missing required expires option")
	}
	key, err := parseKey(opts.PrivateKey)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\n", opts.Method)
	fmt.Fprintf(h, "%s\n", opts.MD5)
	fmt.Fprintf(h, "%s\n", opts.ContentType)
	fmt.Fprintf(h, "%d\n", opts.Expires.Unix())
	fmt.Fprintf(h, "%s", strings.Join(opts.Headers, "\n"))
	fmt.Fprintf(h, "/%s/%s", bucket, name)
	b, err := rsa.SignPKCS1v15(
		rand.Reader,
		key,
		crypto.SHA256,
		h.Sum(nil),
	)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	u := &url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   fmt.Sprintf("/%s/%s", bucket, name),
	}
	q := u.Query()
	q.Set("GoogleAccessId", opts.GoogleAccessID)
	q.Set("Expires", fmt.Sprintf("%d", opts.Expires.Unix()))
	q.Set("Signature", string(encoded))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Attrs returns meta information about the specified object.
func (o *Object) Attrs(ctx context.Context) (*ObjectAttrs, error) {
	obj, err := o.c.raw.Objects.Get(o.bucket, o.object).Projection("full").Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExist
	}
	if err != nil {
		return nil, err
	}
	return newObject(obj), nil
}

// Update updates an object with the provided attributes.
// All zero-value attributes are ignored.
func (o *Object) Update(ctx context.Context, attrs ObjectAttrs) (*ObjectAttrs, error) {
	obj, err := o.c.raw.Objects.Patch(o.bucket, o.object, attrs.toRawObject(o.bucket)).Projection("full").Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExist
	}
	if err != nil {
		return nil, err
	}
	return newObject(obj), nil
}

// Delete deletes the single specified object.
func (o *Object) Delete(ctx context.Context) error {
	return o.c.raw.Objects.Delete(o.bucket, o.object).Do()
}

// CopyObject copies the source object to the destination.
// The copied object's attributes are overwritten by attrs if non-nil.
func (c *Client) CopyObject(ctx context.Context, srcBucket, srcName string, destBucket, destName string, attrs *ObjectAttrs) (*ObjectAttrs, error) {
	if srcBucket == "" || destBucket == "" {
		return nil, errors.New("storage: srcBucket and destBucket must both be non-empty")
	}
	if srcName == "" || destName == "" {
		return nil, errors.New("storage: srcName and destName must be non-empty")
	}
	var rawObject *raw.Object
	if attrs != nil {
		attrs.Name = destName
		if attrs.ContentType == "" {
			return nil, errors.New("storage: attrs.ContentType must be non-empty")
		}
		rawObject = attrs.toRawObject(destBucket)
	}
	o, err := c.raw.Objects.Copy(
		srcBucket, srcName, destBucket, destName, rawObject).Projection("full").Do()
	if err != nil {
		return nil, err
	}
	return newObject(o), nil
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object.
func (o *Object) NewReader(ctx context.Context) (io.ReadCloser, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   fmt.Sprintf("/%s/%s", o.bucket, o.object),
	}
	res, err := o.c.conn.Get(u.String())
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, ErrObjectNotExist
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		res.Body.Close()
		return res.Body, fmt.Errorf("storage: can't read object %v/%v, status code: %v", o.bucket, o.object, res.Status)
	}
	return res.Body, nil
}

// NewWriter returns a storage Writer that writes to the GCS object
// identified by the specified name.
// If such an object doesn't exist, it creates one.
// Attributes can be set on the object by modifying the returned Writer's
// ObjectAttrs field before the first call to Write. The name parameter to this
// function is ignored if the Name field of the ObjectAttrs field is set to a
// non-empty string.
//
// It is the caller's responsibility to call Close when writing is done.
//
// The object is not available and any previous object with the same
// name is not replaced on Cloud Storage until Close is called.
func (o *Object) NewWriter(ctx context.Context) *Writer {
	return &Writer{
		client: o.c,
		bucket: o.bucket,
		name:   o.object,
		donec:  make(chan struct{}),
	}
}

// parseKey converts the binary contents of a private key file
// to an *rsa.PrivateKey. It detects whether the private key is in a
// PEM container or not. If so, it extracts the the private key
// from PEM container before conversion. It only supports PEM
// containers with no passphrase.
func parseKey(key []byte) (*rsa.PrivateKey, error) {
	if block, _ := pem.Decode(key); block != nil {
		key = block.Bytes
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, err
		}
	}
	parsed, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("oauth2: private key is invalid")
	}
	return parsed, nil
}
