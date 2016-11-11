// Copyright 2016 Google Inc. All Rights Reserved.
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

// Package translate is a client for the Google Translate API.
// See https://cloud.google.com/translate for details.
//
// This package is experimental and subject to change without notice.
package translate

import (
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	"golang.org/x/text/language"
	gtransport "google.golang.org/api/googleapi/transport"
	raw "google.golang.org/api/translate/v2"
)

const userAgent = "gcloud-golang-translate/20161029"

// Client is a client for the translate API.
type Client struct {
	raw *raw.Service
}

// NewClient constructs a new Client that can perform Translate operations.
//
// You can find or create API key for your project from the Credentials page of
// the Developers Console (console.developers.google.com).
func NewClient(ctx context.Context, apiKey string) (*Client, error) {
	// Construct a special HTTP client that understands API keys. We don't
	// need OAuth2 support.
	hc := &http.Client{
		Transport: &gtransport.APIKey{
			Key:       apiKey,
			Transport: http.DefaultTransport,
		},
	}
	rawService, err := raw.New(hc)
	if err != nil {
		return nil, fmt.Errorf("translate client: %v", err)
	}
	rawService.UserAgent = userAgent
	return &Client{raw: rawService}, nil
}

// Close closes any resources held by the client.
// Close should be called when the client is no longer needed.
// It need not be called at program exit.
func (c *Client) Close() error { return nil }

// Translate one or more strings of text from a source language to a target
// language. All inputs must be in the same language.
//
// The target parameter supplies the language to translate to. The supported
// languages are listed at
// https://cloud.google.com/translate/v2/translate-reference#supported_languages.
// You can also call the SupportedLanguages method.
//
// The returned Translations appear in the same order as the inputs.
func (c *Client) Translate(ctx context.Context, inputs []string, target language.Tag, opts *Options) ([]Translation, error) {
	call := c.raw.Translations.List(inputs, target.String()).Context(ctx)
	if opts != nil {
		if s := opts.Source; s != language.Und {
			call.Source(s.String())
		}
		if f := opts.Format; f != "" {
			call.Format(f)
		}
	}
	res, err := call.Do()
	if err != nil {
		return nil, err
	}
	var ts []Translation
	for _, t := range res.Translations {
		var source language.Tag
		if t.DetectedSourceLanguage != "" {
			source, err = language.Parse(t.DetectedSourceLanguage)
			if err != nil {
				return nil, err
			}
		}
		ts = append(ts, Translation{
			Text:   t.TranslatedText,
			Source: source,
		})
	}
	return ts, nil
}

// Options contains options for Translate.
type Options struct {
	// Source is the language of the input strings. If empty, the service will
	// attempt to identify the source language automatically and return it within
	// the response.
	Source language.Tag

	// Format describes the format of the input texts. The choices are HTML or
	// Text. The default is HTML.
	Format string
}

// Constants for Options.Format.
const (
	HTML string = "html"
	Text string = "text"
)

// A Translation contains the results of translating a piece of text.
type Translation struct {
	// Text is the input text translated into the target language.
	Text string

	// Source is the detected language of the input text, if source was
	// not supplied to Client.Translate. If source was supplied, this field
	// will be empty.
	Source language.Tag
}

// DetectLanguage attempts to determine the language of the inputs. Each input
// string may be in a different language.
//
// Each slice of Detections in the return value corresponds with one input
// string. A slice of Detections holds multiple hypotheses for the language of
// a single input string.
func (c *Client) DetectLanguage(ctx context.Context, inputs []string) ([][]Detection, error) {
	call := c.raw.Detections.List(inputs).Context(ctx)
	res, err := call.Do()
	if err != nil {
		return nil, err
	}
	var result [][]Detection
	for _, raws := range res.Detections {
		var ds []Detection
		for _, rd := range raws {
			tag, err := language.Parse(rd.Language)
			if err != nil {
				return nil, err
			}
			ds = append(ds, Detection{
				Language:   tag,
				Confidence: rd.Confidence,
				IsReliable: rd.IsReliable,
			})
		}
		result = append(result, ds)
	}
	return result, nil
}

// Detection represents information about a language detected in an input.
type Detection struct {
	// Language is the code of the language detected.
	Language language.Tag

	// Confidence is a number from 0 to 1, with higher numbers indicating more
	// confidence in the detection.
	Confidence float64

	// IsReliable indicates whether the language detection result is reliable.
	IsReliable bool
}

// SupportedLanguages returns a list of supported languages for translation.
// The target parameter is the language to use to return localized, human
// readable names of supported languages.
func (c *Client) SupportedLanguages(ctx context.Context, target language.Tag) ([]Language, error) {
	call := c.raw.Languages.List().Context(ctx).Target(target.String())
	res, err := call.Do()
	if err != nil {
		return nil, err
	}
	var ls []Language
	for _, l := range res.Languages {
		tag, err := language.Parse(l.Language)
		if err != nil {
			return nil, err
		}
		ls = append(ls, Language{
			Name: l.Name,
			Tag:  tag,
		})
	}
	return ls, nil
}

// A Language describes a language supported for translation.
type Language struct {
	// Name is the human-readable name of the language.
	Name string

	// Tag is a standard code for the language.
	Tag language.Tag
}
