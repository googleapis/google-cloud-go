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

package genai

import (
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// WithUseREST is an option that may be passed to a Vertex AI client on
// creation. If true, the client will use REST for transport; if false (default),
// the client will use gRPC for transport.
func WithUseREST(b bool) option.ClientOption {
	return &withUseREST{useREST: b}
}

func (w *withUseREST) ApplyVertexaiOpt(c *config) {
	c.useREST = w.useREST
}

type config struct {
	// useREST uses REST as the underlying transport for the client.
	useREST bool
}

// newConfig generates a new config with all the given
// vertexaiClientOptions applied.
func newConfig(opts ...option.ClientOption) config {
	var conf config
	for _, opt := range opts {
		if vOpt, ok := opt.(vertexaiClientOption); ok {
			vOpt.ApplyVertexaiOpt(&conf)
		}
	}
	return conf
}

// A vertexaiClientOption is an option for a vertexai client.
type vertexaiClientOption interface {
	option.ClientOption
	ApplyVertexaiOpt(*config)
}

type withUseREST struct {
	internaloption.EmbeddableAdapter
	useREST bool
}
