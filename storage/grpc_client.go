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
	"io"
	"os"

	"cloud.google.com/go/internal/trace"
	gapic "cloud.google.com/go/storage/internal/apiv2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
	storagepb "google.golang.org/genproto/googleapis/storage/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
)

const (
	// defaultConnPoolSize is the default number of connections
	// to initialize in the GAPIC gRPC connection pool. A larger
	// connection pool may be necessary for jobs that require
	// high throughput and/or leverage many concurrent streams.
	//
	// This is an experimental API and not intended for public use.
	defaultConnPoolSize = 4

	// globalProjectAlias is the project ID alias used for global buckets.
	//
	// This is only used for the gRPC API.
	globalProjectAlias = "_"
)

// defaultGRPCOptions returns a set of the default client options
// for gRPC client initialization.
//
// This is an experimental API and not intended for public use.
func defaultGRPCOptions() []option.ClientOption {
	defaults := []option.ClientOption{
		option.WithGRPCConnectionPool(defaultConnPoolSize),
	}

	// Set emulator options for gRPC if an emulator was specified. Note that in a
	// hybrid client, STORAGE_EMULATOR_HOST will set the host to use for HTTP and
	// STORAGE_EMULATOR_HOST_GRPC will set the host to use for gRPC (when using a
	// local emulator, HTTP and gRPC must use different ports, so this is
	// necessary).
	//
	// TODO: When the newHybridClient is not longer used, remove
	// STORAGE_EMULATOR_HOST_GRPC and use STORAGE_EMULATOR_HOST for both the
	// HTTP and gRPC based clients.
	if host := os.Getenv("STORAGE_EMULATOR_HOST_GRPC"); host != "" {
		// Strip the scheme from the emulator host. WithEndpoint does not take a
		// scheme for gRPC.
		host = stripScheme(host)

		defaults = append(defaults,
			option.WithEndpoint(host),
			option.WithGRPCDialOption(grpc.WithInsecure()),
			option.WithoutAuthentication(),
		)
	}

	return defaults
}

// grpcStorageClient is the gRPC API implementation of the transport-agnostic
// storageClient interface.
//
// This is an experimental API and not intended for public use.
type grpcStorageClient struct {
	raw      *gapic.Client
	settings *settings
}

// newGRPCStorageClient initializes a new storageClient that uses the gRPC
// Storage API.
//
// This is an experimental API and not intended for public use.
func newGRPCStorageClient(ctx context.Context, opts ...storageOption) (storageClient, error) {
	s := initSettings(opts...)
	s.clientOption = append(defaultGRPCOptions(), s.clientOption...)

	g, err := gapic.NewClient(ctx, s.clientOption...)
	if err != nil {
		return nil, err
	}

	return &grpcStorageClient{
		raw:      g,
		settings: s,
	}, nil
}

func (c *grpcStorageClient) Close() error {
	return c.raw.Close()
}

// Top-level methods.

func (c *grpcStorageClient) GetServiceAccount(ctx context.Context, project string, opts ...storageOption) (string, error) {
	s := callSettings(c.settings, opts...)
	req := &storagepb.GetServiceAccountRequest{
		Project: toProjectResource(project),
	}
	var resp *storagepb.ServiceAccount
	err := run(ctx, func() error {
		var err error
		resp, err = c.raw.GetServiceAccount(ctx, req, s.gax...)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
	if err != nil {
		return "", err
	}
	return resp.EmailAddress, err
}

func (c *grpcStorageClient) CreateBucket(ctx context.Context, project string, attrs *BucketAttrs, opts ...storageOption) (*BucketAttrs, error) {
	s := callSettings(c.settings, opts...)
	b := attrs.toProtoBucket()

	// If there is lifecycle information but no location, explicitly set
	// the location. This is a GCS quirk/bug.
	if b.GetLocation() == "" && b.GetLifecycle() != nil {
		b.Location = "US"
	}

	req := &storagepb.CreateBucketRequest{
		Parent:                     toProjectResource(project),
		Bucket:                     b,
		BucketId:                   b.GetName(),
		PredefinedAcl:              attrs.PredefinedACL,
		PredefinedDefaultObjectAcl: attrs.PredefinedDefaultObjectACL,
	}

	var battrs *BucketAttrs
	err := run(ctx, func() error {
		res, err := c.raw.CreateBucket(ctx, req, s.gax...)

		battrs = newBucketFromProto(res)

		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))

	return battrs, err
}

func (c *grpcStorageClient) ListBuckets(ctx context.Context, project string, opts ...storageOption) *BucketIterator {
	s := callSettings(c.settings, opts...)
	it := &BucketIterator{
		ctx:       ctx,
		projectID: project,
	}

	var gitr *gapic.BucketIterator
	fetch := func(pageSize int, pageToken string) (token string, err error) {
		// Initialize GAPIC-based iterator when pageToken is empty, which
		// indicates that this fetch call is attempting to get the first page.
		//
		// Note: Initializing the GAPIC-based iterator lazily is necessary to
		// capture the BucketIterator.Prefix set by the user *after* the
		// BucketIterator is returned to them from the veneer.
		if pageToken == "" {
			req := &storagepb.ListBucketsRequest{
				Parent: toProjectResource(it.projectID),
				Prefix: it.Prefix,
			}
			gitr = c.raw.ListBuckets(it.ctx, req, s.gax...)
		}

		var buckets []*storagepb.Bucket
		var next string
		err = run(it.ctx, func() error {
			buckets, next, err = gitr.InternalFetch(pageSize, pageToken)
			return err
		}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
		if err != nil {
			return "", err
		}

		for _, bkt := range buckets {
			b := newBucketFromProto(bkt)
			it.buckets = append(it.buckets, b)
		}

		return next, nil
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		fetch,
		func() int { return len(it.buckets) },
		func() interface{} { b := it.buckets; it.buckets = nil; return b })

	return it
}

// Bucket methods.

func (c *grpcStorageClient) DeleteBucket(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) error {
	s := callSettings(c.settings, opts...)
	req := &storagepb.DeleteBucketRequest{
		Name: bucketResourceName(globalProjectAlias, bucket),
	}
	if err := applyBucketCondsProto("grpcStorageClient.DeleteBucket", conds, req); err != nil {
		return err
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}

	return run(ctx, func() error {
		return c.raw.DeleteBucket(ctx, req, s.gax...)
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
}

func (c *grpcStorageClient) GetBucket(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) (*BucketAttrs, error) {
	s := callSettings(c.settings, opts...)
	req := &storagepb.GetBucketRequest{
		Name: bucketResourceName(globalProjectAlias, bucket),
	}
	if err := applyBucketCondsProto("grpcStorageClient.GetBucket", conds, req); err != nil {
		return nil, err
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}

	var battrs *BucketAttrs
	err := run(ctx, func() error {
		res, err := c.raw.GetBucket(ctx, req, s.gax...)

		battrs = newBucketFromProto(res)

		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))

	if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
		return nil, ErrBucketNotExist
	}

	return battrs, err
}
func (c *grpcStorageClient) UpdateBucket(ctx context.Context, bucket string, uattrs *BucketAttrsToUpdate, conds *BucketConditions, opts ...storageOption) (*BucketAttrs, error) {
	s := callSettings(c.settings, opts...)
	b := uattrs.toProtoBucket()
	b.Name = bucketResourceName(globalProjectAlias, bucket)
	req := &storagepb.UpdateBucketRequest{
		Bucket:                     b,
		PredefinedAcl:              uattrs.PredefinedACL,
		PredefinedDefaultObjectAcl: uattrs.PredefinedDefaultObjectACL,
	}
	if err := applyBucketCondsProto("grpcStorageClient.UpdateBucket", conds, req); err != nil {
		return nil, err
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}

	var paths []string
	fieldMask := &fieldmaskpb.FieldMask{
		Paths: paths,
	}
	if uattrs.CORS != nil {
		fieldMask.Paths = append(fieldMask.Paths, "cors")
	}
	if uattrs.DefaultEventBasedHold != nil {
		fieldMask.Paths = append(fieldMask.Paths, "default_event_based_hold")
	}
	if uattrs.RetentionPolicy != nil {
		fieldMask.Paths = append(fieldMask.Paths, "retention_policy")
	}
	if uattrs.VersioningEnabled != nil {
		fieldMask.Paths = append(fieldMask.Paths, "versioning")
	}
	if uattrs.RequesterPays != nil {
		fieldMask.Paths = append(fieldMask.Paths, "billing")
	}
	if uattrs.BucketPolicyOnly != nil || uattrs.UniformBucketLevelAccess != nil || uattrs.PublicAccessPrevention != PublicAccessPreventionUnknown {
		fieldMask.Paths = append(fieldMask.Paths, "iam_config")
	}
	if uattrs.Encryption != nil {
		fieldMask.Paths = append(fieldMask.Paths, "encryption")
	}
	if uattrs.Lifecycle != nil {
		fieldMask.Paths = append(fieldMask.Paths, "lifecycle")
	}
	if uattrs.Logging != nil {
		fieldMask.Paths = append(fieldMask.Paths, "logging")
	}
	if uattrs.Website != nil {
		fieldMask.Paths = append(fieldMask.Paths, "website")
	}
	if uattrs.PredefinedACL != "" {
		// In cases where PredefinedACL is set, Acl is cleared.
		fieldMask.Paths = append(fieldMask.Paths, "acl")
	}
	if uattrs.PredefinedDefaultObjectACL != "" {
		// In cases where PredefinedDefaultObjectACL is set, DefaultObjectAcl is cleared.
		fieldMask.Paths = append(fieldMask.Paths, "default_object_acl")
	}
	if uattrs.acl != nil {
		// In cases where acl is set by UpdateBucketACL method.
		fieldMask.Paths = append(fieldMask.Paths, "acl")
	}
	if uattrs.defaultObjectACL != nil {
		// In cases where defaultObjectACL is set by UpdateBucketACL method.
		fieldMask.Paths = append(fieldMask.Paths, "default_object_acl")
	}
	if uattrs.StorageClass != "" {
		fieldMask.Paths = append(fieldMask.Paths, "storage_class")
	}
	if uattrs.RPO != RPOUnknown {
		fieldMask.Paths = append(fieldMask.Paths, "rpo")
	}
	// TODO(cathyo): Handle labels. Pending b/230510191.
	req.UpdateMask = fieldMask

	var battrs *BucketAttrs
	err := run(ctx, func() error {
		res, err := c.raw.UpdateBucket(ctx, req, s.gax...)
		battrs = newBucketFromProto(res)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))

	return battrs, err
}
func (c *grpcStorageClient) LockBucketRetentionPolicy(ctx context.Context, bucket string, conds *BucketConditions, opts ...storageOption) error {
	s := callSettings(c.settings, opts...)
	req := &storagepb.LockBucketRetentionPolicyRequest{
		Bucket: bucketResourceName(globalProjectAlias, bucket),
	}
	if err := applyBucketCondsProto("grpcStorageClient.LockBucketRetentionPolicy", conds, req); err != nil {
		return err
	}

	return run(ctx, func() error {
		_, err := c.raw.LockBucketRetentionPolicy(ctx, req, s.gax...)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))

}
func (c *grpcStorageClient) ListObjects(ctx context.Context, bucket string, q *Query, opts ...storageOption) *ObjectIterator {
	s := callSettings(c.settings, opts...)
	it := &ObjectIterator{
		ctx: ctx,
	}
	if q != nil {
		it.query = *q
	}
	req := &storagepb.ListObjectsRequest{
		Parent:             bucketResourceName(globalProjectAlias, bucket),
		Prefix:             it.query.Prefix,
		Delimiter:          it.query.Delimiter,
		Versions:           it.query.Versions,
		LexicographicStart: it.query.StartOffset,
		LexicographicEnd:   it.query.EndOffset,
		// TODO(noahietz): Convert a projection to a FieldMask.
		// ReadMask: q.Projection,
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}
	gitr := c.raw.ListObjects(it.ctx, req, s.gax...)
	fetch := func(pageSize int, pageToken string) (token string, err error) {
		var objects []*storagepb.Object
		err = run(it.ctx, func() error {
			objects, token, err = gitr.InternalFetch(pageSize, pageToken)
			return err
		}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				err = ErrBucketNotExist
			}
			return "", err
		}

		for _, obj := range objects {
			b := newObjectFromProto(obj)
			it.items = append(it.items, b)
		}

		return token, nil
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })

	return it
}

// Object metadata methods.

func (c *grpcStorageClient) DeleteObject(ctx context.Context, bucket, object string, gen int64, conds *Conditions, opts ...storageOption) error {
	s := callSettings(c.settings, opts...)
	req := &storagepb.DeleteObjectRequest{
		Bucket: bucketResourceName(globalProjectAlias, bucket),
		Object: object,
	}
	if err := applyCondsProto("grpcStorageClient.DeleteObject", gen, conds, req); err != nil {
		return err
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}
	err := run(ctx, func() error {
		return c.raw.DeleteObject(ctx, req, s.gax...)
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
	if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
		return ErrObjectNotExist
	}
	return err
}

func (c *grpcStorageClient) GetObject(ctx context.Context, bucket, object string, gen int64, encryptionKey []byte, conds *Conditions, opts ...storageOption) (*ObjectAttrs, error) {
	s := callSettings(c.settings, opts...)
	req := &storagepb.GetObjectRequest{
		Bucket: bucketResourceName(globalProjectAlias, bucket),
		Object: object,
	}
	if err := applyCondsProto("grpcStorageClient.GetObject", gen, conds, req); err != nil {
		return nil, err
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}
	if encryptionKey != nil {
		req.CommonObjectRequestParams = toProtoCommonObjectRequestParams(encryptionKey)
	}

	var attrs *ObjectAttrs
	err := run(ctx, func() error {
		res, err := c.raw.GetObject(ctx, req, s.gax...)
		attrs = newObjectFromProto(res)

		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))

	if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
		return nil, ErrObjectNotExist
	}

	return attrs, err
}

func (c *grpcStorageClient) UpdateObject(ctx context.Context, bucket, object string, uattrs *ObjectAttrsToUpdate, gen int64, encryptionKey []byte, conds *Conditions, opts ...storageOption) (*ObjectAttrs, error) {
	s := callSettings(c.settings, opts...)
	o := uattrs.toProtoObject(bucketResourceName(globalProjectAlias, bucket), object)
	req := &storagepb.UpdateObjectRequest{
		Object: o,
	}
	if err := applyCondsProto("grpcStorageClient.UpdateObject", gen, conds, req); err != nil {
		return nil, err
	}
	if s.userProject != "" {
		ctx = setUserProjectMetadata(ctx, s.userProject)
	}
	if encryptionKey != nil {
		req.CommonObjectRequestParams = toProtoCommonObjectRequestParams(encryptionKey)
	}

	var paths []string
	fieldMask := &fieldmaskpb.FieldMask{
		Paths: paths,
	}
	if uattrs.EventBasedHold != nil {
		fieldMask.Paths = append(fieldMask.Paths, "event_based_hold")
	}
	if uattrs.TemporaryHold != nil {
		fieldMask.Paths = append(fieldMask.Paths, "temporary_hold")
	}
	if uattrs.ContentType != nil {
		fieldMask.Paths = append(fieldMask.Paths, "content_type")
	}
	if uattrs.ContentLanguage != nil {
		fieldMask.Paths = append(fieldMask.Paths, "content_language")
	}
	if uattrs.ContentEncoding != nil {
		fieldMask.Paths = append(fieldMask.Paths, "content_encoding")
	}
	if uattrs.ContentDisposition != nil {
		fieldMask.Paths = append(fieldMask.Paths, "content_disposition")
	}
	if uattrs.CacheControl != nil {
		fieldMask.Paths = append(fieldMask.Paths, "cache_control")
	}
	if !uattrs.CustomTime.IsZero() {
		fieldMask.Paths = append(fieldMask.Paths, "custom_time")
	}

	// TODO(cathyo): Handle ACL and PredefinedACL. Pending b/233617896.
	// TODO(cathyo): Handle metadata. Pending b/230510191.

	req.UpdateMask = fieldMask

	var attrs *ObjectAttrs
	err := run(ctx, func() error {
		res, err := c.raw.UpdateObject(ctx, req, s.gax...)
		attrs = newObjectFromProto(res)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
	if e, ok := status.FromError(err); ok && e.Code() == codes.NotFound {
		return nil, ErrObjectNotExist
	}

	return attrs, err
}

// Default Object ACL methods.

func (c *grpcStorageClient) DeleteDefaultObjectACL(ctx context.Context, bucket string, entity ACLEntity, opts ...storageOption) error {
	// There is no separate API for PATCH in gRPC.
	// Make a GET call first to retrieve BucketAttrs.
	attrs, err := c.GetBucket(ctx, bucket, nil, opts...)
	if err != nil {
		return err
	}
	// Delete the entity and copy other remaining ACL entities.
	var acl []ACLRule
	for _, a := range attrs.DefaultObjectACL {
		if a.Entity != entity {
			acl = append(acl, a)
		}
	}
	uattrs := &BucketAttrsToUpdate{defaultObjectACL: acl}
	// Call UpdateBucket with a MetagenerationMatch precondition set.
	if _, err = c.UpdateBucket(ctx, bucket, uattrs, &BucketConditions{MetagenerationMatch: attrs.MetaGeneration}, opts...); err != nil {
		return err
	}
	return nil
}
func (c *grpcStorageClient) ListDefaultObjectACLs(ctx context.Context, bucket string, opts ...storageOption) ([]ACLRule, error) {
	attrs, err := c.GetBucket(ctx, bucket, nil, opts...)
	if err != nil {
		return nil, err
	}
	return attrs.DefaultObjectACL, nil
}
func (c *grpcStorageClient) UpdateDefaultObjectACL(ctx context.Context, opts ...storageOption) (*ACLRule, error) {
	return nil, errMethodNotSupported
}

// Bucket ACL methods.

func (c *grpcStorageClient) DeleteBucketACL(ctx context.Context, bucket string, entity ACLEntity, opts ...storageOption) error {
	// There is no separate API for PATCH in gRPC.
	// Make a GET call first to retrieve BucketAttrs.
	attrs, err := c.GetBucket(ctx, bucket, nil, opts...)
	if err != nil {
		return err
	}
	// Delete the entity and copy other remaining ACL entities.
	var acl []ACLRule
	for _, a := range attrs.ACL {
		if a.Entity != entity {
			acl = append(acl, a)
		}
	}
	uattrs := &BucketAttrsToUpdate{acl: acl}
	// Call UpdateBucket with a MetagenerationMatch precondition set.
	if _, err = c.UpdateBucket(ctx, bucket, uattrs, &BucketConditions{MetagenerationMatch: attrs.MetaGeneration}, opts...); err != nil {
		return err
	}
	return nil
}
func (c *grpcStorageClient) ListBucketACLs(ctx context.Context, bucket string, opts ...storageOption) ([]ACLRule, error) {
	attrs, err := c.GetBucket(ctx, bucket, nil, opts...)
	if err != nil {
		return nil, err
	}
	return attrs.ACL, nil
}

func (c *grpcStorageClient) UpdateBucketACL(ctx context.Context, bucket string, entity ACLEntity, role ACLRole, opts ...storageOption) (*ACLRule, error) {
	// There is no separate API for PATCH in gRPC.
	// Make a GET call first to retrieve BucketAttrs.
	attrs, err := c.GetBucket(ctx, bucket, nil, opts...)
	if err != nil {
		return nil, err
	}
	var acl []ACLRule
	aclRule := ACLRule{Entity: entity, Role: role}
	acl = append(attrs.ACL, aclRule)
	uattrs := &BucketAttrsToUpdate{acl: acl}
	// Call UpdateBucket with a MetagenerationMatch precondition set.
	_, err = c.UpdateBucket(ctx, bucket, uattrs, &BucketConditions{MetagenerationMatch: attrs.MetaGeneration}, opts...)
	if err != nil {
		return nil, err
	}
	return &aclRule, err
}

// Object ACL methods.

func (c *grpcStorageClient) DeleteObjectACL(ctx context.Context, bucket, object string, entity ACLEntity, opts ...storageOption) error {
	return errMethodNotSupported
}
func (c *grpcStorageClient) ListObjectACLs(ctx context.Context, bucket, object string, opts ...storageOption) ([]ACLRule, error) {
	return nil, errMethodNotSupported
}
func (c *grpcStorageClient) UpdateObjectACL(ctx context.Context, bucket, object string, entity ACLEntity, role ACLRole, opts ...storageOption) (*ACLRule, error) {
	return nil, errMethodNotSupported
}

// Media operations.

func (c *grpcStorageClient) ComposeObject(ctx context.Context, req *composeObjectRequest, opts ...storageOption) (*ObjectAttrs, error) {
	return nil, errMethodNotSupported
}
func (c *grpcStorageClient) RewriteObject(ctx context.Context, req *rewriteObjectRequest, opts ...storageOption) (*rewriteObjectResponse, error) {
	return nil, errMethodNotSupported
}

func (c *grpcStorageClient) NewRangeReader(ctx context.Context, params *newRangeReaderParams, opts ...storageOption) (r *Reader, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/storage.grpcStorageClient.NewRangeReader")
	defer func() { trace.EndSpan(ctx, err) }()

	if params.conds != nil {
		if err := params.conds.validate("grpcStorageClient.NewRangeReader"); err != nil {
			return nil, err
		}
	}

	s := callSettings(c.settings, opts...)

	// A negative length means "read to the end of the object", but the
	// read_limit field it corresponds to uses zero to mean the same thing. Thus
	// we coerce the length to 0 to read to the end of the object.
	if params.length < 0 {
		params.length = 0
	}

	b := bucketResourceName(globalProjectAlias, params.bucket)
	// TODO(noahdietz): Use encryptionKey to set relevant request fields.
	req := &storagepb.ReadObjectRequest{
		Bucket: b,
		Object: params.object,
	}
	// The default is a negative value, which means latest.
	if params.gen >= 0 {
		req.Generation = params.gen
	}

	// Define a function that initiates a Read with offset and length, assuming
	// we have already read seen bytes.
	reopen := func(seen int64) (*readStreamResponse, context.CancelFunc, error) {
		// If the context has already expired, return immediately without making
		// we call.
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		cc, cancel := context.WithCancel(ctx)

		start := params.offset + seen
		// Only set a ReadLimit if length is greater than zero, because zero
		// means read it all.
		if params.length > 0 {
			req.ReadLimit = params.length - seen
		}
		req.ReadOffset = start

		if err := applyCondsProto("gRPCReader.reopen", params.gen, params.conds, req); err != nil {
			cancel()
			return nil, nil, err
		}

		var stream storagepb.Storage_ReadObjectClient
		var msg *storagepb.ReadObjectResponse
		var err error

		err = run(cc, func() error {
			stream, err = c.raw.ReadObject(cc, req, s.gax...)
			if err != nil {
				return err
			}

			msg, err = stream.Recv()

			return err
		}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
		if err != nil {
			// Close the stream context we just created to ensure we don't leak
			// resources.
			cancel()
			return nil, nil, err
		}

		return &readStreamResponse{stream, msg}, cancel, nil
	}

	res, cancel, err := reopen(0)
	if err != nil {
		return nil, err
	}

	// The first message was Recv'd on stream open, use it to populate the
	// object metadata.
	msg := res.response
	obj := msg.GetMetadata()
	// This is the size of the entire object, even if only a range was requested.
	size := obj.GetSize()

	r = &Reader{
		Attrs: ReaderObjectAttrs{
			Size:            size,
			ContentType:     obj.GetContentType(),
			ContentEncoding: obj.GetContentEncoding(),
			CacheControl:    obj.GetCacheControl(),
			LastModified:    obj.GetUpdateTime().AsTime(),
			Metageneration:  obj.GetMetageneration(),
			Generation:      obj.GetGeneration(),
		},
		reader: &gRPCReader{
			stream: res.stream,
			reopen: reopen,
			cancel: cancel,
			size:   size,
			// Store the content from the first Recv in the
			// client buffer for reading later.
			leftovers: msg.GetChecksummedData().GetContent(),
		},
	}

	cr := msg.GetContentRange()
	if cr != nil {
		r.Attrs.StartOffset = cr.GetStart()
		r.remain = cr.GetEnd() - cr.GetStart() + 1
	} else {
		r.remain = size
	}

	// Only support checksums when reading an entire object, not a range.
	if checksums := msg.GetObjectChecksums(); checksums != nil && checksums.Crc32C != nil && params.offset == 0 && params.length == 0 {
		r.wantCRC = checksums.GetCrc32C()
		r.checkCRC = true
	}

	return r, nil
}

func (c *grpcStorageClient) OpenWriter(ctx context.Context, w *Writer, opts ...storageOption) error {
	return errMethodNotSupported
}

// IAM methods.

func (c *grpcStorageClient) GetIamPolicy(ctx context.Context, resource string, version int32, opts ...storageOption) (*iampb.Policy, error) {
	// TODO: Need a way to set UserProject, potentially in X-Goog-User-Project system parameter.
	s := callSettings(c.settings, opts...)
	req := &iampb.GetIamPolicyRequest{
		Resource: bucketResourceName(globalProjectAlias, resource),
		Options: &iampb.GetPolicyOptions{
			RequestedPolicyVersion: version,
		},
	}
	var rp *iampb.Policy
	err := run(ctx, func() error {
		var err error
		rp, err = c.raw.GetIamPolicy(ctx, req, s.gax...)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))

	return rp, err
}

func (c *grpcStorageClient) SetIamPolicy(ctx context.Context, resource string, policy *iampb.Policy, opts ...storageOption) error {
	// TODO: Need a way to set UserProject, potentially in X-Goog-User-Project system parameter.
	s := callSettings(c.settings, opts...)

	req := &iampb.SetIamPolicyRequest{
		Resource: bucketResourceName(globalProjectAlias, resource),
		Policy:   policy,
	}

	return run(ctx, func() error {
		_, err := c.raw.SetIamPolicy(ctx, req, s.gax...)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
}

func (c *grpcStorageClient) TestIamPermissions(ctx context.Context, resource string, permissions []string, opts ...storageOption) ([]string, error) {
	// TODO: Need a way to set UserProject, potentially in X-Goog-User-Project system parameter.
	s := callSettings(c.settings, opts...)
	req := &iampb.TestIamPermissionsRequest{
		Resource:    bucketResourceName(globalProjectAlias, resource),
		Permissions: permissions,
	}
	var res *iampb.TestIamPermissionsResponse
	err := run(ctx, func() error {
		var err error
		res, err = c.raw.TestIamPermissions(ctx, req, s.gax...)
		return err
	}, s.retry, s.idempotent, setRetryHeaderGRPC(ctx))
	if err != nil {
		return nil, err
	}
	return res.Permissions, nil
}

// HMAC Key methods.

func (c *grpcStorageClient) GetHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) (*HMACKey, error) {
	return nil, errMethodNotSupported
}
func (c *grpcStorageClient) ListHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) *HMACKeysIterator {
	return &HMACKeysIterator{}
}
func (c *grpcStorageClient) UpdateHMACKey(ctx context.Context, desc *hmacKeyDesc, attrs *HMACKeyAttrsToUpdate, opts ...storageOption) (*HMACKey, error) {
	return nil, errMethodNotSupported
}
func (c *grpcStorageClient) CreateHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) (*HMACKey, error) {
	return nil, errMethodNotSupported
}
func (c *grpcStorageClient) DeleteHMACKey(ctx context.Context, desc *hmacKeyDesc, opts ...storageOption) error {
	return errMethodNotSupported
}

// setUserProjectMetadata appends a project ID to the outgoing Context metadata
// via the x-goog-user-project system parameter defined at
// https://cloud.google.com/apis/docs/system-parameters. This is only for
// billing purposes, and is generally optional, except for requester-pays
// buckets.
func setUserProjectMetadata(ctx context.Context, project string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-goog-user-project", project)
}

type readStreamResponse struct {
	stream   storagepb.Storage_ReadObjectClient
	response *storagepb.ReadObjectResponse
}

type gRPCReader struct {
	seen, size int64
	stream     storagepb.Storage_ReadObjectClient
	reopen     func(seen int64) (*readStreamResponse, context.CancelFunc, error)
	leftovers  []byte
	cancel     context.CancelFunc
}

// Read reads bytes into the user's buffer from an open gRPC stream.
func (r *gRPCReader) Read(p []byte) (int, error) {
	// No stream to read from, either never initiliazed or Close was called.
	// Note: There is a potential concurrency issue if multiple routines are
	// using the same reader. One encounters an error and the stream is closed
	// and then reopened while the other routine attempts to read from it.
	if r.stream == nil {
		return 0, fmt.Errorf("reader has been closed")
	}

	// The entire object has been read by this reader, return EOF.
	if r.size != 0 && r.size == r.seen {
		return 0, io.EOF
	}

	var n int
	// Read leftovers and return what was available to conform to the Reader
	// interface: https://pkg.go.dev/io#Reader.
	if len(r.leftovers) > 0 {
		n = copy(p, r.leftovers)
		r.seen += int64(n)
		r.leftovers = r.leftovers[n:]
		return n, nil
	}

	// Attempt to Recv the next message on the stream.
	msg, err := r.recv()
	if err != nil {
		return 0, err
	}

	// TODO: Determine if we need to capture incremental CRC32C for this
	// chunk. The Object CRC32C checksum is captured when directed to read
	// the entire Object. If directed to read a range, we may need to
	// calculate the range's checksum for verification if the checksum is
	// present in the response here.
	// TODO: Figure out if we need to support decompressive transcoding
	// https://cloud.google.com/storage/docs/transcoding.
	content := msg.GetChecksummedData().GetContent()
	n = copy(p[n:], content)
	leftover := len(content) - n
	if leftover > 0 {
		// Wasn't able to copy all of the data in the message, store for
		// future Read calls.
		r.leftovers = content[n:]
	}
	r.seen += int64(n)

	return n, nil
}

// Close cancels the read stream's context in order for it to be closed and
// collected.
func (r *gRPCReader) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	r.stream = nil
	return nil
}

// recv attempts to Recv the next message on the stream. In the event
// that a retryable error is encountered, the stream will be closed, reopened,
// and Recv again. This will attempt to Recv until one of the following is true:
//
// * Recv is successful
// * A non-retryable error is encountered
// * The Reader's context is canceled
//
// The last error received is the one that is returned, which could be from
// an attempt to reopen the stream.
//
// This is an experimental API and not intended for public use.
func (r *gRPCReader) recv() (*storagepb.ReadObjectResponse, error) {
	msg, err := r.stream.Recv()
	if err != nil && shouldRetry(err) {
		// This will "close" the existing stream and immediately attempt to
		// reopen the stream, but will backoff if further attempts are necessary.
		// Reopening the stream Recvs the first message, so if retrying is
		// successful, the next logical chunk will be returned.
		msg, err = r.reopenStream()
	}

	return msg, err
}

// reopenStream "closes" the existing stream and attempts to reopen a stream and
// sets the Reader's stream and cancelStream properties in the process.
//
// This is an experimental API and not intended for public use.
func (r *gRPCReader) reopenStream() (*storagepb.ReadObjectResponse, error) {
	// Close existing stream and initialize new stream with updated offset.
	r.Close()

	res, cancel, err := r.reopen(r.seen)
	if err != nil {
		return nil, err
	}
	r.stream = res.stream
	r.cancel = cancel
	return res.response, nil
}
