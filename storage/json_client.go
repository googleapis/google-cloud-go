// Copyright 2022 Google LLC
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
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	raw "google.golang.org/api/storage/v1"
	"google.golang.org/api/transport"
	htransport "google.golang.org/api/transport/http"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
)

// jsonStorageClient is the HTTP-JSON API implementation of the transport-agnostic
// storageClient interface.
//
// Note: This is experimental and should not be used in production yet.
type jsonStorageClient struct {
	creds    *google.Credentials
	hc       *http.Client
	host     string
	raw      *raw.Service
	scheme   string
	settings *settings
}

// newJSONStorageClient initializes a new storageClient that uses the HTTP-JSON
// Storage API.
//
// Note: This is experimental and should not be used in production yet.
func newJSONStorageClient(ctx context.Context, opts ...storageOption) (storageClient, error) {
	s := initSettings(opts...)
	o := s.clientOption

	var creds *google.Credentials
	// In general, it is recommended to use raw.NewService instead of htransport.NewClient
	// since raw.NewService configures the correct default endpoints when initializing the
	// internal http client. However, in our case, "NewRangeReader" in reader.go needs to
	// access the http client directly to make requests, so we create the client manually
	// here so it can be re-used by both reader.go and raw.NewService. This means we need to
	// manually configure the default endpoint options on the http client. Furthermore, we
	// need to account for STORAGE_EMULATOR_HOST override when setting the default endpoints.
	if host := os.Getenv("STORAGE_EMULATOR_HOST"); host == "" {
		// Prepend default options to avoid overriding options passed by the user.
		o = append([]option.ClientOption{option.WithScopes(ScopeFullControl, "https://www.googleapis.com/auth/cloud-platform"), option.WithUserAgent(userAgent)}, o...)

		o = append(o, internaloption.WithDefaultEndpoint("https://storage.googleapis.com/storage/v1/"))
		o = append(o, internaloption.WithDefaultMTLSEndpoint("https://storage.mtls.googleapis.com/storage/v1/"))

		// Don't error out here. The user may have passed in their own HTTP
		// client which does not auth with ADC or other common conventions.
		c, err := transport.Creds(ctx, o...)
		if err == nil {
			creds = c
			o = append(o, internaloption.WithCredentials(creds))
		}
	} else {
		var hostURL *url.URL

		if strings.Contains(host, "://") {
			h, err := url.Parse(host)
			if err != nil {
				return nil, err
			}
			hostURL = h
		} else {
			// Add scheme for user if not supplied in STORAGE_EMULATOR_HOST
			// URL is only parsed correctly if it has a scheme, so we build it ourselves
			hostURL = &url.URL{Scheme: "http", Host: host}
		}

		hostURL.Path = "storage/v1/"
		endpoint := hostURL.String()

		// Append the emulator host as default endpoint for the user
		o = append([]option.ClientOption{option.WithoutAuthentication()}, o...)

		o = append(o, internaloption.WithDefaultEndpoint(endpoint))
		o = append(o, internaloption.WithDefaultMTLSEndpoint(endpoint))
	}
	s.clientOption = o

	// htransport selects the correct endpoint among WithEndpoint (user override), WithDefaultEndpoint, and WithDefaultMTLSEndpoint.
	hc, ep, err := htransport.NewClient(ctx, s.clientOption...)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}
	// RawService should be created with the chosen endpoint to take account of user override.
	rawService, err := raw.NewService(ctx, option.WithEndpoint(ep), option.WithHTTPClient(hc))
	if err != nil {
		return nil, fmt.Errorf("storage client: %v", err)
	}
	// Update readHost and scheme with the chosen endpoint.
	u, err := url.Parse(ep)
	if err != nil {
		return nil, fmt.Errorf("supplied endpoint %q is not valid: %v", ep, err)
	}

	return &jsonStorageClient{
		creds:    creds,
		hc:       hc,
		host:     u.Host,
		raw:      rawService,
		scheme:   u.Scheme,
		settings: s,
	}, nil
}

// Top-level methods.

func (c *jsonStorageClient) GetServiceAccount(ctx context.Context, project string, opts ...storageOption) (string, error) {
	return "", ErrMethodNotSupported
}
func (c *jsonStorageClient) CreateBucket(ctx context.Context, project string, attrs *BucketAttrs, opts ...storageOption) (*BucketAttrs, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) ListBuckets(ctx context.Context, project string, opts ...storageOption) (*BucketIterator, error) {
	return nil, ErrMethodNotSupported
}

// Bucket methods.

func (c *jsonStorageClient) DeleteBucket(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) GetBucket(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) (*BucketAttrs, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) UpdateBucket(ctx context.Context, uattrs *BucketAttrsToUpdate, conds *BucketConditions, opts ...storageOption) (*BucketAttrs, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) LockBucketRetentionPolicy(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) ListObjects(ctx context.Context, bucket string, q *Query, opts ...storageOption) (*ObjectIterator, error) {
	return nil, ErrMethodNotSupported
}

// Object metadata methods.

func (c *jsonStorageClient) DeleteObject(ctx context.Context, bucket, object string, conds *Conditions, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) GetObject(ctx context.Context, bucket, object string, conds *Conditions, opts ...storageOption) (*ObjectAttrs, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) UpdateObject(ctx context.Context, bucket, object string, uattrs *ObjectAttrsToUpdate, conds *Conditions, opts ...storageOption) (*ObjectAttrs, error) {
	return nil, ErrMethodNotSupported
}

// Default Object ACL methods.

func (c *jsonStorageClient) DeleteDefaultObjectACL(ctx context.Context, bucket string, entity ACLEntity, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) ListDefaultObjectACLs(ctx context.Context, bucket string, opts ...storageOption) ([]ACLRule, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) UpdateDefaultObjectACL(ctx context.Context, opts ...storageOption) (*ACLRule, error) {
	return nil, ErrMethodNotSupported
}

// Bucket ACL methods.

func (c *jsonStorageClient) DeleteBucketACL(ctx context.Context, bucket string, entity ACLEntity, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) ListBucketACLs(ctx context.Context, bucket string, opts ...storageOption) ([]ACLRule, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) UpdateBucketACL(ctx context.Context, bucket string, entity ACLEntity, role ACLRole, opts ...storageOption) (*ACLRule, error) {
	return nil, ErrMethodNotSupported
}

// Object ACL methods.

func (c *jsonStorageClient) DeleteObjectACL(ctx context.Context, bucket, object string, entity ACLEntity, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) ListObjectACLs(ctx context.Context, bucket, object string, opts ...storageOption) ([]ACLRule, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) UpdateObjectACL(ctx context.Context, bucket, object string, entity ACLEntity, role ACLRole, opts ...storageOption) (*ACLRule, error) {
	return nil, ErrMethodNotSupported
}

// Media operations.

func (c *jsonStorageClient) ComposeObject(ctx context.Context, req *composeObjectRequest, opts ...storageOption) (*ObjectAttrs, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) RewriteObject(ctx context.Context, req *rewriteObjectRequest, opts ...storageOption) (*rewriteObjectResponse, error) {
	return nil, ErrMethodNotSupported
}

func (c *jsonStorageClient) OpenReader(ctx context.Context, r *Reader, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) OpenWriter(ctx context.Context, w *Writer, opts ...storageOption) error {
	return ErrMethodNotSupported
}

// IAM methods.

func (c *jsonStorageClient) GetIamPolicy(ctx context.Context, resource string, version int32, opts ...storageOption) (*iampb.Policy, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) SetIamPolicy(ctx context.Context, resource string, policy *iampb.Policy, opts ...storageOption) error {
	return ErrMethodNotSupported
}
func (c *jsonStorageClient) TestIamPermissions(ctx context.Context, resource string, permissions []string, opts ...storageOption) ([]string, error) {
	return nil, ErrMethodNotSupported
}

// HMAC Key methods.

func (c *jsonStorageClient) GetHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) (*HMACKey, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) ListHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) *HMACKeysIterator {
	return nil
}
func (c *jsonStorageClient) UpdateHMACKey(ctx context.Context, desc *hmacKeyDesc, attrs *HMACKeyAttrsToUpdate, opts ...storageOption) (*HMACKey, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) CreateHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) (*HMACKey, error) {
	return nil, ErrMethodNotSupported
}
func (c *jsonStorageClient) DeleteHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) error {
	return ErrMethodNotSupported
}
