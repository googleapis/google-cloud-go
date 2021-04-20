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

// [START resourcesettings_generated_resourcesettings_apiv1_Client_UpdateSettingValue]

package main

import (
	"context"

	resourcesettings "cloud.google.com/go/resourcesettings/apiv1"
	resourcesettingspb "google.golang.org/genproto/googleapis/cloud/resourcesettings/v1"
)

func main() {
	// import resourcesettingspb "google.golang.org/genproto/googleapis/cloud/resourcesettings/v1"

	ctx := context.Background()
	c, err := resourcesettings.NewClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &resourcesettingspb.UpdateSettingValueRequest{
		// TODO: Fill request struct fields.
	}
	resp, err := c.UpdateSettingValue(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END resourcesettings_generated_resourcesettings_apiv1_Client_UpdateSettingValue]
