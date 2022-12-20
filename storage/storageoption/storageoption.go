// Copyright 2022 Google LLC
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

// Package storageoption contains options specific to the Google Storage client. For
// more options available to Google API clients, see the option package. [link]
package storageoption

import (
	"cloud.google.com/go/storage/internal"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

func WithJSONReads() option.ClientOption {
	return &withReadAPI{useJSON: true}
}

func WithXMLReads() option.ClientOption {
	return &withReadAPI{useJSON: false}
}

type withReadAPI struct {
	internaloption.EmbeddableAdapter
	useJSON bool
}

func (w *withReadAPI) ApplyOpt(c *internal.StorageConfig) {
	c.UseJSONforReads = w.useJSON
	c.ReadAPIWasSet = true
}
