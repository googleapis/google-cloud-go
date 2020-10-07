// Copyright 2020 Google LLC
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

package mock

import (
	"context"
	"testing"

	"github.com/googleapis/gax-go/v2"
	translatepb "google.golang.org/genproto/googleapis/cloud/translate/v3"
)

// mockClient fullfills the TranslationClient interface and is used as a mock
// standin for a `translate.Client` that is only used to TranslateText.
type mockClient struct{}

func (*mockClient) TranslateText(_ context.Context, req *translatepb.TranslateTextRequest, opts ...gax.CallOption) (*translatepb.TranslateTextResponse, error) {
	resp := &translatepb.TranslateTextResponse{
		Translations: []*translatepb.Translation{
			{TranslatedText: "Hello World"},
		},
	}
	return resp, nil
}

func TestTranslateTextWithInterfaceClient(t *testing.T) {
	client := &mockClient{}
	text, err := TranslateTextWithInterfaceClient(client, "Hola Mundo", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello World" {
		t.Fatalf("got %q, want Hello World", text)
	}
}
