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
	"net/http"
	// "os"
	"strings"
	"testing"

	"cloud.google.com/go/vertexai/genai"
	"cloud.google.com/go/vertexai/genai/tokenizer"

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

// corporaGenerator is a helper function that downloads a zip archive from a given URL,
// extracts the content of each file in the archive,
// and returns a slice of corporaInfo objects containing the name and content of each file.
func corporaGenerator(url string) ([]corporaInfo, error) {
	var corpora []corporaInfo

	// Download the zip file
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error downloading file: %v", err)
	}
	defer resp.Body.Close()

	// Read the content of the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	// Create a zip reader from the downloaded content
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("error creating zip reader: %v", err)
	}

	// Iterate over each file in the zip archive
	for _, file := range zipReader.File {
		fileReader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("error opening file: %v", err)
		}

		// Check if the file is a text file
		if !file.FileInfo().IsDir() && file.FileInfo().Mode().IsRegular() {
			content, err := io.ReadAll(fileReader)
			fileReader.Close()
			if err != nil {
				return nil, fmt.Errorf("error reading file content: %v", err)
			}

			corpora = append(corpora, corporaInfo{
				Name:    file.Name[len("udhr/"):],
				Content: content,
			})
		}
	}

	return corpora, nil
}

// udhrCorpus represents the Universal Declaration of Human Rights (UDHR) corpus.
// This corpus contains translations of the UDHR into many languages,
// stored in a specific directory structure within a zip archive.
//
// The files in the corpus USUALLY follow a naming convention:
//
//	<Language>_<Script>-<Encoding>
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
	EncodingByFileSuffix map[string]encoding.Encoding
	EncodingByFilename   map[string]encoding.Encoding

	// Skip lists files that should be skipped during testing.
	// This is useful for excluding files that are known to cause issues
	// or are not relevant for the test.
	Skip map[string]bool
}

// newUdhrCorpus initializes a new udhrCorpus with encoding patterns and skip set
// func newUdhrCorpus() *udhrCorpus {
func newUdhrCorpus() *udhrCorpus {

	EncodingByFileSuffix := map[string]encoding.Encoding{
		"Latin1":   charmap.ISO8859_1,
		"Hebrew":   charmap.ISO8859_8,
		"Arabic":   charmap.Windows1256,
		"UTF8":     encoding.Nop,
		"Cyrillic": charmap.Windows1251,
		"SJIS":     japanese.ShiftJIS,
		"GB2312":   simplifiedchinese.HZGB2312,
		"Latin2":   charmap.ISO8859_2,
		"Greek":    charmap.ISO8859_7,
		"Turkish":  charmap.ISO8859_9,
		"Baltic":   charmap.ISO8859_4,
		"EUC":      japanese.EUCJP,
		"VPS":      charmap.Windows1258,
		"Agra":     encoding.Nop,
		"T61":      charmap.ISO8859_3,
	}

	// For non-conventional filenames:
	EncodingByFilename := map[string]encoding.Encoding{
		"Czech_Cesky-UTF8":              charmap.Windows1250,
		"Polish-Latin2":                 charmap.Windows1250,
		"Polish_Polski-Latin2":          charmap.Windows1250,
		"Amahuaca":                      charmap.ISO8859_1,
		"Turkish_Turkce-Turkish":        charmap.ISO8859_9,
		"Lithuanian_Lietuviskai-Baltic": charmap.ISO8859_4,
		"Abkhaz-Cyrillic+Abkh":          charmap.Windows1251,
		"Azeri_Azerbaijani_Cyrillic-Az.Times.Cyr.Normal0117": charmap.Windows1251,
		"Azeri_Azerbaijani_Latin-Az.Times.Lat0117":           charmap.ISO8859_2,
	}

	// The skip list comes from the NLTK source code which says these are unsupported encodings,
	// or in general encodings Go doesn't support.
	// See NLTK source code reference: https://github.com/nltk/nltk/blob/f6567388b4399000b9aa2a6b0db713bff3fe332a/nltk/corpus/reader/udhr.py#L14
	Skip := map[string]bool{
		// The following files are not fully decodable because they
		// were truncated at wrong bytes:
		"Burmese_Myanmar-UTF8":     true,
		"Japanese_Nihongo-JIS":     true,
		"Chinese_Mandarin-HZ":      true,
		"Chinese_Mandarin-UTF8":    true,
		"Gujarati-UTF8":            true,
		"Hungarian_Magyar-Unicode": true,
		"Lao-UTF8":                 true,
		"Magahi-UTF8":              true,
		"Marathi-UTF8":             true,
		"Tamil-UTF8":               true,
		"Magahi-Agrarpc":           true,
		"Magahi-Agra":              true,
		// encoding not supported in Go.
		"Vietnamese-VIQR": true,
		"Vietnamese-TCVN": true,
		// The following files are encoded for specific fonts:
		"Burmese_Myanmar-WinResearcher":  true,
		"Armenian-DallakHelv":            true,
		"Tigrinya_Tigrigna-VG2Main":      true,
		"Amharic-Afenegus6..60375":       true,
		"Navaho_Dine-Navajo-Navaho-font": true,
		// The following files are unintended:
		"Czech-Latin2-err":     true,
		"Russian_Russky-UTF8~": true,
	}

	return &udhrCorpus{
		EncodingByFileSuffix: EncodingByFileSuffix,
		EncodingByFilename:   EncodingByFilename,
		Skip:                 Skip,
	}
}

// getEncoding returns the encoding for a given filename based on patterns
func (ucr *udhrCorpus) getEncoding(filename string) (encoding.Encoding, bool) {
	if enc, exists := ucr.EncodingByFilename[filename]; exists {
		return enc, true
	}

	parts := strings.Split(filename, "-")
	encodingKey := parts[len(parts)-1]
	if enc, exists := ucr.EncodingByFileSuffix[encodingKey]; exists {
		return enc, true
	}

	return nil, false
}

// shouldSkip checks if the file should be skipped
func (ucr *udhrCorpus) shouldSkip(filename string) bool {
	return ucr.Skip[filename]
}

// decodeBytes decodes the given byte slice using the specified encoding
func decodeBytes(enc encoding.Encoding, content []byte) (string, error) {
	decodedBytes, _, err := transform.Bytes(enc.NewDecoder(), content)
	if err != nil {
		return "", fmt.Errorf("error decoding bytes: %v", err)
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
	ucr := newUdhrCorpus()

	corporaURL := "https://raw.githubusercontent.com/nltk/nltk_data/gh-pages/packages/corpora/udhr.zip"
	corporaFiles, err := corporaGenerator(corporaURL)
	if err != nil {
		t.Fatalf("Failed to generate corpora: %v", err)
	}

	// Create channels for work distribution and results collection
	corporaChan := make(chan corporaInfo)
	doneChan := make(chan bool)

	worker := func() {
		for corpora := range corporaChan {
			if ucr.shouldSkip(corpora.Name) {
				fmt.Printf("Skipping file: %s\n", corpora.Name)
				continue
			}

			enc, found := ucr.getEncoding(corpora.Name)
			if !found {
				fmt.Printf("No encoding found for file: %s\n", corpora.Name)
				continue
			}

			decodedContent, err := decodeBytes(enc, corpora.Content)
			if err != nil {
				log.Fatalf("Failed to decode bytes: %v", err)
			}

			tok, err := tokenizer.New(defaultModel)
			if err != nil {
				log.Fatal(err)
			}

			localNtoks, err := tok.CountTokens(genai.Text(decodedContent))
			if err != nil {
				log.Fatal(err)
			}
			remoteNtoks, err := model.CountTokens(ctx, genai.Text(decodedContent))
			if err != nil {
				log.Fatal(corpora.Name, err)
			}
			if localNtoks.TotalTokens != remoteNtoks.TotalTokens {
				t.Errorf("expected %d(remote count-token results), but got %d(local count-token results)", remoteNtoks, localNtoks)
			}
		}
		doneChan <- true
	}

	const numWorkers = 10
	for i := 0; i < numWorkers; i++ {
		go worker()
	}

	for _, corporaInfo := range corporaFiles {
		corporaChan <- corporaInfo
	}
	close(corporaChan)

	for i := 0; i < numWorkers; i++ {
		<-doneChan
	}

}
