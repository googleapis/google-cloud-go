// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secretmanager

import (
	"context"

	"cloud.google.com/go/iam"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
)

// IAM returns a handle to inspect and change permissions of the resource
// indicated by the given resource path. Name should be of the format
// `projects/my-project/secrets/my-secret`.
func (c *Client) IAM(name string) *iam.Handle {
	return iam.InternalNewHandleClient(&iamClient{c}, name)
}

// iamClient implements the Get/Set/Test IAM methods.
type iamClient struct {
	c *Client
}

func (c *iamClient) Get(ctx context.Context, resource string) (*iampb.Policy, error) {
	return c.c.GetIamPolicy(ctx, &iampb.GetIamPolicyRequest{
		Resource: resource,
	})
}

func (c *iamClient) Set(ctx context.Context, resource string, p *iampb.Policy) error {
	_, err := c.c.SetIamPolicy(ctx, &iampb.SetIamPolicyRequest{
		Policy:   p,
		Resource: resource,
	})
	return err
}

func (c *iamClient) Test(ctx context.Context, resource string, perms []string) ([]string, error) {
	resp, err := c.c.TestIamPermissions(ctx, &iampb.TestIamPermissionsRequest{
		Resource:    resource,
		Permissions: perms,
	})
	if err != nil {
		return nil, err
	}
	return resp.Permissions, nil
}
