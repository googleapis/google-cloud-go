// Copyright 2025 Google LLC
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

// This is handwritten code to avoid breaking changes.

package admin

import (
	"context"
	"fmt"
	"math"
	"net/url"

	adminpb "cloud.google.com/go/iam/admin/apiv1/adminpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"
)

func (c *iamGRPCClient) ListRolesIter(ctx context.Context, req *adminpb.ListRolesRequest, opts ...gax.CallOption) *RoleIterator {
	hds := []string{"x-goog-request-params", fmt.Sprintf("%s=%v", "parent", url.QueryEscape(req.GetParent()))}

	hds = append(c.xGoogHeaders, hds...)
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, hds...)
	opts = append((*c.CallOptions).ListRoles[0:len((*c.CallOptions).ListRoles):len((*c.CallOptions).ListRoles)], opts...)
	it := &RoleIterator{}
	req = proto.Clone(req).(*adminpb.ListRolesRequest)
	it.InternalFetch = func(pageSize int, pageToken string) ([]*adminpb.Role, string, error) {
		resp := &adminpb.ListRolesResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			var err error
			resp, err = executeRPC(ctx, c.iamClient.ListRoles, req, settings.GRPC, c.logger, "ListRoles")
			return err
		}, opts...)
		if err != nil {
			return nil, "", err
		}

		it.Response = resp
		return resp.GetRoles(), resp.GetNextPageToken(), nil
	}
	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

func (c *iamGRPCClient) QueryGrantableRolesIter(ctx context.Context, req *adminpb.QueryGrantableRolesRequest, opts ...gax.CallOption) *RoleIterator {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).QueryGrantableRoles[0:len((*c.CallOptions).QueryGrantableRoles):len((*c.CallOptions).QueryGrantableRoles)], opts...)
	it := &RoleIterator{}
	req = proto.Clone(req).(*adminpb.QueryGrantableRolesRequest)
	it.InternalFetch = func(pageSize int, pageToken string) ([]*adminpb.Role, string, error) {
		resp := &adminpb.QueryGrantableRolesResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			var err error
			resp, err = executeRPC(ctx, c.iamClient.QueryGrantableRoles, req, settings.GRPC, c.logger, "QueryGrantableRoles")
			return err
		}, opts...)
		if err != nil {
			return nil, "", err
		}

		it.Response = resp
		return resp.GetRoles(), resp.GetNextPageToken(), nil
	}
	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

func (c *iamGRPCClient) QueryTestablePermissionsIter(ctx context.Context, req *adminpb.QueryTestablePermissionsRequest, opts ...gax.CallOption) *PermissionIterator {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).QueryTestablePermissions[0:len((*c.CallOptions).QueryTestablePermissions):len((*c.CallOptions).QueryTestablePermissions)], opts...)
	it := &PermissionIterator{}
	req = proto.Clone(req).(*adminpb.QueryTestablePermissionsRequest)
	it.InternalFetch = func(pageSize int, pageToken string) ([]*adminpb.Permission, string, error) {
		resp := &adminpb.QueryTestablePermissionsResponse{}
		if pageToken != "" {
			req.PageToken = pageToken
		}
		if pageSize > math.MaxInt32 {
			req.PageSize = math.MaxInt32
		} else if pageSize != 0 {
			req.PageSize = int32(pageSize)
		}
		err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
			var err error
			resp, err = executeRPC(ctx, c.iamClient.QueryTestablePermissions, req, settings.GRPC, c.logger, "QueryTestablePermissions")
			return err
		}, opts...)
		if err != nil {
			return nil, "", err
		}

		it.Response = resp
		return resp.GetPermissions(), resp.GetNextPageToken(), nil
	}
	fetch := func(pageSize int, pageToken string) (string, error) {
		items, nextPageToken, err := it.InternalFetch(pageSize, pageToken)
		if err != nil {
			return "", err
		}
		it.items = append(it.items, items...)
		return nextPageToken, nil
	}

	it.pageInfo, it.nextFunc = iterator.NewPageInfo(fetch, it.bufLen, it.takeBuf)
	it.pageInfo.MaxSize = int(req.GetPageSize())
	it.pageInfo.Token = req.GetPageToken()

	return it
}

// ListRoles lists the Roles defined on a resource.
func (c *IamClient) ListRoles(ctx context.Context, req *adminpb.ListRolesRequest, opts ...gax.CallOption) (*adminpb.ListRolesResponse, error) {
	return c.internalClient.ListRoles(ctx, req, opts...)
}

// QueryGrantableRoles queries roles that can be granted on a particular resource.
// A role is grantable if it can be used as the role in a binding for a policy
// for that resource.
func (c *IamClient) QueryGrantableRoles(ctx context.Context, req *adminpb.QueryGrantableRolesRequest, opts ...gax.CallOption) (*adminpb.QueryGrantableRolesResponse, error) {
	return c.internalClient.QueryGrantableRoles(ctx, req, opts...)
}

// QueryTestablePermissions lists the permissions testable on a resource.
// A permission is testable if it can be tested for an identity on a resource.
func (c *IamClient) QueryTestablePermissions(ctx context.Context, req *adminpb.QueryTestablePermissionsRequest, opts ...gax.CallOption) (*adminpb.QueryTestablePermissionsResponse, error) {
	return c.internalClient.QueryTestablePermissions(ctx, req, opts...)
}

// ListRoles lists the Roles defined on a resource.
func (c *iamGRPCClient) ListRoles(ctx context.Context, req *adminpb.ListRolesRequest, opts ...gax.CallOption) (*adminpb.ListRolesResponse, error) {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).QueryTestablePermissions[0:len((*c.CallOptions).QueryTestablePermissions):len((*c.CallOptions).QueryTestablePermissions)], opts...)
	var resp *adminpb.ListRolesResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = executeRPC(ctx, c.iamClient.ListRoles, req, settings.GRPC, c.logger, "QueryTestablePermissions")
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// QueryGrantableRoles queries roles that can be granted on a particular resource.
// A role is grantable if it can be used as the role in a binding for a policy
// for that resource.
func (c *iamGRPCClient) QueryGrantableRoles(ctx context.Context, req *adminpb.QueryGrantableRolesRequest, opts ...gax.CallOption) (*adminpb.QueryGrantableRolesResponse, error) {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).QueryTestablePermissions[0:len((*c.CallOptions).QueryTestablePermissions):len((*c.CallOptions).QueryTestablePermissions)], opts...)
	var resp *adminpb.QueryGrantableRolesResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = executeRPC(ctx, c.iamClient.QueryGrantableRoles, req, settings.GRPC, c.logger, "QueryTestablePermissions")
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// QueryTestablePermissions lists the permissions testable on a resource.
// A permission is testable if it can be tested for an identity on a resource.
func (c *iamGRPCClient) QueryTestablePermissions(ctx context.Context, req *adminpb.QueryTestablePermissionsRequest, opts ...gax.CallOption) (*adminpb.QueryTestablePermissionsResponse, error) {
	ctx = gax.InsertMetadataIntoOutgoingContext(ctx, c.xGoogHeaders...)
	opts = append((*c.CallOptions).QueryTestablePermissions[0:len((*c.CallOptions).QueryTestablePermissions):len((*c.CallOptions).QueryTestablePermissions)], opts...)
	var resp *adminpb.QueryTestablePermissionsResponse
	err := gax.Invoke(ctx, func(ctx context.Context, settings gax.CallSettings) error {
		var err error
		resp, err = executeRPC(ctx, c.iamClient.QueryTestablePermissions, req, settings.GRPC, c.logger, "QueryTestablePermissions")
		return err
	}, opts...)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
