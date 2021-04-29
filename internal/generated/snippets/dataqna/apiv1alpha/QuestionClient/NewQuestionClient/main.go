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

// [START dataqna_generated_dataqna_apiv1alpha_NewQuestionClient]

package main

import (
	"context"

	dataqna "cloud.google.com/go/dataqna/apiv1alpha"
)

func main() {
	ctx := context.Background()
	c, err := dataqna.NewQuestionClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use client.
	_ = c
}

// [END dataqna_generated_dataqna_apiv1alpha_NewQuestionClient]
