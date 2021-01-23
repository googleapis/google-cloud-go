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
	"fmt"
	"log"
	"os"

	"github.com/googleapis/gax-go/v2"
	translatepb "google.golang.org/genproto/googleapis/cloud/translate/v3"
)

// TranslationClient is used to translate text.
type TranslationClient interface {
	TranslateText(ctx context.Context, req *translatepb.TranslateTextRequest, opts ...gax.CallOption) (*translatepb.TranslateTextResponse, error)
}

// TranslateTextWithInterfaceClient translates text to the targetLand using the
// provided client.
func TranslateTextWithInterfaceClient(client TranslationClient, text string, targetLang string) (string, error) {
	ctx := context.Background()
	log.Printf("Translating %q to %q", text, targetLang)
	req := &translatepb.TranslateTextRequest{
		Parent:             fmt.Sprintf("projects/%s/locations/global", os.Getenv("GOOGLE_CLOUD_PROJECT")),
		TargetLanguageCode: "en-US",
		Contents:           []string{text},
	}
	resp, err := client.TranslateText(ctx, req)
	if err != nil {
		return "", fmt.Errorf("unable to translate text: %v", err)
	}
	translations := resp.GetTranslations()
	if len(translations) != 1 {
		return "", fmt.Errorf("expected only one result, got %d", len(translations))
	}
	return translations[0].TranslatedText, nil
}
