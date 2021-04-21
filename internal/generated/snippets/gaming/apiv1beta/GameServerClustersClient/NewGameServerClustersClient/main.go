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

// [START gameservices_generated_gaming_apiv1beta_NewGameServerClustersClient]

package main

import (
	"context"

	gaming "cloud.google.com/go/gaming/apiv1beta"
)

func main() {
	ctx := context.Background()
	c, err := gaming.NewGameServerClustersClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use client.
	_ = c
}

// [END gameservices_generated_gaming_apiv1beta_NewGameServerClustersClient]
