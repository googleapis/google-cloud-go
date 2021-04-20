// Copyright 2021 Google LLC
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

// [START iam_generated_iam_admin_apiv1_IamClient_ListServiceAccounts]

package main

import (
	"context"

	admin "cloud.google.com/go/iam/admin/apiv1"
	"google.golang.org/api/iterator"
	adminpb "google.golang.org/genproto/googleapis/iam/admin/v1"
)

func main() {
	ctx := context.Background()
	c, err := admin.NewIamClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &adminpb.ListServiceAccountsRequest{
		// TODO: Fill request struct fields.
	}
	it := c.ListServiceAccounts(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		// TODO: Use resp.
		_ = resp
	}
}

// [END iam_generated_iam_admin_apiv1_IamClient_ListServiceAccounts]
