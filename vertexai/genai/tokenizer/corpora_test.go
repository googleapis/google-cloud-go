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
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"cloud.google.com/go/vertexai/genai"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// corporaInfo holds the name and content of a file in the zip archive
type corporaInfo struct {
	Name    string
	Content []byte
}

// corporaGenerator is a generator function that returns a channel to iterate over files in the zip archive
func corporaGenerator(url string) (<-chan corporaInfo, <-chan error) {
	out := make(chan corporaInfo)
	errCh := make(chan error, 1)

	// This function is run as a goroutine because it performs a network request
	// to download the zip file, which can take some time. Running it in a
	// separate goroutine prevents the function from blocking the caller
	// while the download is in progress.
	//
	// The goroutine writes the downloaded file information to the `out` channel
	// and any errors to the `errCh` channel. This allows the caller to process
	// files as they become available and handle errors gracefully.
	go func() {
		defer close(out)
		defer close(errCh)

		// Download the zip file
		resp, err := http.Get(url)
		if err != nil {
			errCh <- fmt.Errorf("error downloading file: %v", err)
			return
		}
		defer resp.Body.Close()

		// Read the content of the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			errCh <- fmt.Errorf("error reading response body: %v", err)
			return
		}

		// Create a zip reader from the downloaded content
		zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			errCh <- fmt.Errorf("error creating zip reader: %v", err)
			return
		}

		// Iterate over each file in the zip archive
		for _, file := range zipReader.File {
			fileReader, err := file.Open()
			if err != nil {
				errCh <- fmt.Errorf("error opening file: %v", err)
				continue
			}

			// Check if the file is a text file
			if !file.FileInfo().IsDir() && file.FileInfo().Mode().IsRegular() {
				content, err := io.ReadAll(fileReader)
				if err != nil {
					errCh <- fmt.Errorf("error reading file: %v", err)
					fileReader.Close()
					continue
				}
				fileReader.Close()

				out <- corporaInfo{
					Name:    file.Name[len("udhr/"):],
					Content: content,
				}
			} else {
				fileReader.Close()
			}
		}
	}()

	return out, errCh
}

// udhrCorpus represents the Universal Declaration of Human Rights (UDHR) corpus.
// This corpus contains translations of the UDHR into many languages,
// stored in a specific directory structure within a zip archive.
//
// The files in the corpus follow a naming convention:
//   <Language>_<Script>-<Encoding>
//
// For example:
//   - English_English-UTF8
//   - French_Français-Latin1
//   - Spanish_Español-UTF8
//
// The Language and Script parts are self-explanatory.
// The Encoding part indicates the character encoding used in the file.
//
// This corpus is used to test the token counting functionality
// against a diverse set of languages and encodings.
type udhrCorpus struct {
	// Encodings maps regular expressions to character encodings.
	// These patterns are used to determine the encoding of a file
	// based on its filename.
	Encodings []encodingPattern

	// Skip lists files that should be skipped during testing.
	// This is useful for excluding files that are known to cause issues
	// or are not relevant for the test.
	Skip      map[string]bool
}

// encodingPattern struct to hold regex pattern and corresponding encoding
type encodingPattern struct {
	Pattern  *regexp.Regexp
	Encoding encoding.Encoding
}

// newUdhrCorpus initializes a new udhrCorpus with encoding patterns and skip set
func newUdhrCorpus() *udhrCorpus {
	encodings := []encodingPattern{
		{Pattern: regexp.MustCompile(".*-Latin1$"), Encoding: charmap.ISO8859_1},
		{Pattern: regexp.MustCompile(".*-Hebrew$"), Encoding: charmap.ISO8859_8},
		{Pattern: regexp.MustCompile(".*-Arabic$"), Encoding: charmap.Windows1256},
		{Pattern: regexp.MustCompile("Czech_Cesky-UTF8"), Encoding: charmap.Windows1250},
		{Pattern: regexp.MustCompile("Polish-Latin2"), Encoding: charmap.Windows1250},
		{Pattern: regexp.MustCompile("Polish_Polski-Latin2"), Encoding: charmap.Windows1250},
		{Pattern: regexp.MustCompile(".*-Cyrillic$"), Encoding: charmap.Windows1251},
		{Pattern: regexp.MustCompile(".*-SJIS$"), Encoding: japanese.ShiftJIS},
		{Pattern: regexp.MustCompile(".*-GB2312$"), Encoding: simplifiedchinese.HZGB2312},
		{Pattern: regexp.MustCompile(".*-Latin2$"), Encoding: charmap.ISO8859_2},
		{Pattern: regexp.MustCompile(".*-Greek$"), Encoding: charmap.ISO8859_7},
		{Pattern: regexp.MustCompile(".*-UTF8$"), Encoding: encoding.Nop}, // No transformation needed
		{Pattern: regexp.MustCompile("Amahuaca"), Encoding: charmap.ISO8859_1},
		{Pattern: regexp.MustCompile("Turkish_Turkce-Turkish"), Encoding: charmap.ISO8859_9},
		{Pattern: regexp.MustCompile("Lithuanian_Lietuviskai-Baltic"), Encoding: charmap.ISO8859_4},
		{Pattern: regexp.MustCompile("Japanese_Nihongo-EUC"), Encoding: japanese.EUCJP},
		{Pattern: regexp.MustCompile(`Abkhaz\-Cyrillic\+Abkh`), Encoding: charmap.Windows1251},
	}

	skip := map[string]bool{
		"Burmese_Myanmar-UTF8":           true,
		"Japanese_Nihongo-JIS":           true,
		"Chinese_Mandarin-HZ":            true,
		"Chinese_Mandarin-UTF8":          true,
		"Gujarati-UTF8":                  true,
		"Hungarian_Magyar-Unicode":       true,
		"Lao-UTF8":                       true,
		"Magahi-UTF8":                    true,
		"Marathi-UTF8":                   true,
		"Tamil-UTF8":                     true,
		"Vietnamese-VPS":                 true,
		"Vietnamese-VIQR":                true,
		"Vietnamese-TCVN":                true,
		"Magahi-Agra":                    true,
		"Bhojpuri-Agra":                  true,
		"Esperanto-T61":                  true,
		"Burmese_Myanmar-WinResearcher":  true,
		"Armenian-DallakHelv":            true,
		"Tigrinya_Tigrigna-VG2Main":      true,
		"Amharic-Afenegus6..60375":       true,
		"Navaho_Dine-Navajo-Navaho-font": true,
		"Azeri_Azerbaijani_Cyrillic-Az.Times.Cyr.Normal0117": true,
		"Azeri_Azerbaijani_Latin-Az.Times.Lat0117":           true,
		"Czech-Latin2-err":     true,
		"Russian_Russky-UTF8~": true,
	}

	return &udhrCorpus{
		Encodings: encodings,
		Skip:      skip,
	}
}

// getEncoding returns the encoding for a given filename based on patterns
func (ucr *udhrCorpus) getEncoding(filename string) (encoding.Encoding, bool) {
	for _, pattern := range ucr.Encodings {
		if pattern.Pattern.MatchString(filename) {
			return pattern.Encoding, true
		}
	}
	return nil, false
}

// shouldSkip checks if the file should be skipped
func (ucr *udhrCorpus) shouldSkip(filename string) bool {
	return ucr.Skip[filename]
}

// decodeBytes converts a byte slice from a specified encoding to a UTF-8 string
func decodeBytes(enc encoding.Encoding, data []byte) (string, error) {
	if enc == encoding.Nop {
		return string(data), nil
	}

	if enc == nil {
		return "", fmt.Errorf("unsupported encoding or custom handling required")
	}

	decoder := enc.NewDecoder()
	reader := transform.NewReader(strings.NewReader(string(data)), decoder)
	decodedBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("error decoding data: %v", err)
	}

	return string(decodedBytes), nil
}

const defaultModel = "gemini-1.0-pro"
const defaultLocation = "us-central1"

func TestCountTokensWithCorpora(t *testing.T) {
	projectID := os.Getenv("VERTEX_PROJECT_ID")
	if testing.Short() {
		t.Skip("skipping live test in -short mode")
	}

	if projectID == "" {
		t.Skip("set a VERTEX_PROJECT_ID env var to run live tests")
	}
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, defaultLocation)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	model := client.GenerativeModel(defaultModel)

	t.Run("RemoteAndLocalCountTokensTest", func(t *testing.T) {
		corporaUrl := "https://raw.githubusercontent.com/nltk/nltk_data/gh-pages/packages/corpora/udhr.zip"
		fileCh, errCh := corporaGenerator(corporaUrl)
		ucr := newUdhrCorpus()

		// Iterate over files generated by the generator function
		for fileInfo := range fileCh {
			if ucr.shouldSkip(fileInfo.Name) {
				fmt.Printf("Skipping file: %s\n", fileInfo.Name)
				continue
			}

			enc, found := ucr.getEncoding(fileInfo.Name)
			if !found {
				fmt.Printf("No encoding found for file: %s\n", fileInfo.Name)
				continue
			}

			decodedContent, err := decodeBytes(enc, fileInfo.Content)
			if err != nil {
				log.Fatalf("Failed to decode bytes: %v", err)
			}

			tok, err := New("gemini-1.5-flash")
			if err != nil {
				log.Fatal(err)
			}

			localNtoks, err := tok.CountTokens(genai.Text(decodedContent))
			if err != nil {
				log.Fatal(err)
			}
			remoteNtoks, err := model.CountTokens(ctx, genai.Text(decodedContent))
			if err != nil {
				log.Fatal(fileInfo.Name, err)
			}
			if localNtoks.TotalTokens != remoteNtoks.TotalTokens {
				t.Errorf("expected %d(remote count-token results), but got %d(local count-token results)", remoteNtoks, localNtoks)
			}

		}

		if err := <-errCh; err != nil {
			fmt.Println("Error:", err)
		}
	})
}
