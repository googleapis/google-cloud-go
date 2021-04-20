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

// [START texttospeech_generated_texttospeech_apiv1_Client_ListVoices]

package main

import (
	"context"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	texttospeechpb "google.golang.org/genproto/googleapis/cloud/texttospeech/v1"
)

func main() {
	// import texttospeechpb "google.golang.org/genproto/googleapis/cloud/texttospeech/v1"

	ctx := context.Background()
	c, err := texttospeech.NewClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}

	req := &texttospeechpb.ListVoicesRequest{
		// TODO: Fill request struct fields.
	}
	resp, err := c.ListVoices(ctx, req)
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use resp.
	_ = resp
}

// [END texttospeech_generated_texttospeech_apiv1_Client_ListVoices]
