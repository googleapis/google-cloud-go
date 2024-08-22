// Copyright 2024 Google LLC
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

// Package tokenizer provides local token counting for Gemini models. This
// tokenizer downloads its model from the web, but otherwise doesn't require
// an API call for every [CountTokens] invocation.
package tokenizer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"cloud.google.com/go/vertexai/genai"
	"cloud.google.com/go/vertexai/internal/sentencepiece"
)

var supportedModels = map[string]bool{
	"gemini-1.0-pro":       true,
	"gemini-1.5-pro":       true,
	"gemini-1.5-flash":     true,
	"gemini-1.0-pro-001":   true,
	"gemini-1.0-pro-002":   true,
	"gemini-1.5-pro-001":   true,
	"gemini-1.5-flash-001": true,
}

// Tokenizer is a local tokenizer for text.
type Tokenizer struct {
	processor *sentencepiece.Processor
}

// CountTokensResponse is the response of [Tokenizer.CountTokens].
type CountTokensResponse struct {
	TotalTokens int32
}

// New creates a new [Tokenizer] from a model name; the model name is the same
// as you would pass to a [genai.Client.GenerativeModel].
func New(modelName string) (*Tokenizer, error) {
	if !supportedModels[modelName] {
		return nil, fmt.Errorf("model %s is not supported", modelName)
	}

	data, err := loadModelData(gemmaModelURL, gemmaModelHash)
	if err != nil {
		return nil, fmt.Errorf("loading model: %w", err)
	}

	processor, err := sentencepiece.NewProcessor(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating processor: %w", err)
	}

	return &Tokenizer{processor: processor}, nil
}

// CountTokens counts the tokens in all the given parts and returns their
// sum. Only [genai.Text] parts are suppored; an error will be returned if
// non-text parts are provided.
func (tok *Tokenizer) CountTokens(parts ...genai.Part) (*CountTokensResponse, error) {
	sum := 0

	for _, part := range parts {
		if t, ok := part.(genai.Text); ok {
			toks := tok.processor.Encode(string(t))
			sum += len(toks)
		} else {
			return nil, fmt.Errorf("Tokenizer.CountTokens only supports Text parts")
		}
	}

	return &CountTokensResponse{TotalTokens: int32(sum)}, nil
}

// gemmaModelURL is the URL from which we download the model file.
const gemmaModelURL = "https://raw.githubusercontent.com/google/gemma_pytorch/33b652c465537c6158f9a472ea5700e5e770ad3f/tokenizer/tokenizer.model"

// gemmaModelHash is the expected hash of the model file (as calculated
// by [hashString]).
const gemmaModelHash = "61a7b147390c64585d6c3543dd6fc636906c9af3865a5548f27f31aee1d4c8e2"

// downloadModelFile downloads a file from the given URL.
func downloadModelFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// hashString computes a hex string of the SHA256 hash of data.
func hashString(data []byte) string {
	hash256 := sha256.Sum256(data)
	return hex.EncodeToString(hash256[:])
}

// loadModelData loads model data from the given URL, using a local file-system
// cache. wantHash is the hash (as returned by [hashString] expected on the
// loaded data.
//
// Caching logic:
//
// Assuming $TEMP_DIR is the temporary directory used by the OS, this function
// uses the file $TEMP_DIR/vertexai_tokenizer_model/$urlhash as a cache, where
// $urlhash is hashString(url).
//
// If this cache file doesn't exist, or the data it contains doesn't match
// wantHash, downloads data from the URL and writes it into the cache. If the
// URL's data doesn't match the hash, an error is returned.
func loadModelData(url string, wantHash string) ([]byte, error) {
	urlhash := hashString([]byte(url))
	cacheDir := filepath.Join(os.TempDir(), "vertexai_tokenizer_model")
	cachePath := filepath.Join(cacheDir, urlhash)

	cacheData, err := os.ReadFile(cachePath)
	if err != nil || hashString(cacheData) != wantHash {
		cacheData, err = downloadModelFile(url)
		if err != nil {
			return nil, fmt.Errorf("loading cache and downloading model: %w", err)
		}

		if hashString(cacheData) != wantHash {
			return nil, fmt.Errorf("downloaded model hash mismatch")
		}

		err = os.MkdirAll(cacheDir, 0770)
		if err != nil {
			return nil, fmt.Errorf("creating cache dir: %w", err)
		}
		err = os.WriteFile(cachePath, cacheData, 0660)
		if err != nil {
			return nil, fmt.Errorf("writing cache file: %w", err)
		}
	}

	return cacheData, nil
}
