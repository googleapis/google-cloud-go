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

// [START servicedirectory_generated_servicedirectory_apiv1beta1_RegistrationClient_DeleteNamespace]

package main

import (
	"context"

	servicedirectory "cloud.google.com/go/servicedirectory/apiv1beta1"
	servicedirectorypb "google.golang.org/genproto/googleapis/cloud/servicedirectory/v1beta1"
)

func main() {
	ctx := context.Background()
	c, err := servicedirectory.NewRegistrationClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &servicedirectorypb.DeleteNamespaceRequest{
		// TODO: Fill request struct fields.
	}
	err = c.DeleteNamespace(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
}

// [END servicedirectory_generated_servicedirectory_apiv1beta1_RegistrationClient_DeleteNamespace]
