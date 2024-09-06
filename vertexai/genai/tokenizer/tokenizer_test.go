// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package tokenizer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/vertexai/genai"
)

func TestDownload(t *testing.T) {
	b, err := downloadModelFile(gemmaModelURL)
	if err != nil {
		t.Fatal(err)
	}

	if hashString(b) != gemmaModelHash {
		t.Errorf("gemma model hash doesn't match")
	}
}

func TestLoadModelData(t *testing.T) {
	// Tests that loadModelData manages to load the model properly, and download
	// a new one as needed.
	checkDataAndErr := func(data []byte, err error) {
		t.Helper()
		if err != nil {
			t.Error(err)
		}
		gotHash := hashString(data)
		if gotHash != gemmaModelHash {
			t.Errorf("got hash=%v, want=%v", gotHash, gemmaModelHash)
		}
	}

	data, err := loadModelData(gemmaModelURL, gemmaModelHash)
	checkDataAndErr(data, err)

	// The cache should exist now and have the right data, try again.
	data, err = loadModelData(gemmaModelURL, gemmaModelHash)
	checkDataAndErr(data, err)

	// Overwrite cache file with wrong data, and try again.
	cacheDir := filepath.Join(os.TempDir(), "vertexai_tokenizer_model")
	cachePath := filepath.Join(cacheDir, hashString([]byte(gemmaModelURL)))
	_ = os.MkdirAll(cacheDir, 0770)
	_ = os.WriteFile(cachePath, []byte{0, 1, 2, 3}, 0660)
	data, err = loadModelData(gemmaModelURL, gemmaModelHash)
	checkDataAndErr(data, err)
}

func TestCreateTokenizer(t *testing.T) {
	// Create a tokenizer successfully
	_, err := New("gemini-1.5-flash")
	if err != nil {
		t.Error(err)
	}

	// Create a tokenizer with an unsupported model
	_, err = New("gemini-0.92")
	if err == nil {
		t.Errorf("got no error, want error")
	}
}

func TestCountTokens(t *testing.T) {
	var tests = []struct {
		parts     []genai.Part
		wantCount int32
	}{
		{[]genai.Part{genai.Text("hello world")}, 2},
		{[]genai.Part{genai.Text("<table><th></th></table>")}, 4},
		{[]genai.Part{genai.Text("hello world"), genai.Text("<table><th></th></table>")}, 6},
	}

	tok, err := New("gemini-1.5-flash")
	if err != nil {
		t.Error(err)
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := tok.CountTokens(tt.parts...)
			if err != nil {
				t.Error(err)
			}
			if got.TotalTokens != tt.wantCount {
				t.Errorf("got %v, want %v", got.TotalTokens, tt.wantCount)
			}
		})
	}
}

func TestCountTokensNonText(t *testing.T) {
	tok, err := New("gemini-1.5-flash")
	if err != nil {
		t.Error(err)
	}

	_, err = tok.CountTokens(genai.Text("foo"), genai.ImageData("format", []byte{0, 1}))
	if err == nil {
		t.Error("got no error, want error")
	}
}
