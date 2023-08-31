// Copyright 2023 Google LLC
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

package transport

import (
	"context"
	"net/http"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

func TestSetAuthHeader(t *testing.T) {
	tests := []struct {
		name string
		tp   auth.TokenProvider
	}{
		{
			name: "basic success",
			tp:   staticProvider{&auth.Token{Value: "abc123", Type: "Bearer"}},
		},
		{
			name: "missing type",
			tp:   staticProvider{&auth.Token{Value: "abc123"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := SetAuthHeader(context.Background(), tt.tp, req); err != nil {
				t.Fatal(err)
			}
			tok, err := tt.tp.Token(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if want := internal.TokenTypeBearer + " " + tok.Value; req.Header.Get(authHeaderKey) != want {
				t.Errorf("got %q, want %q", req.Header.Get(authHeaderKey), want)
			}
		})
	}
}

type staticProvider struct {
	t *auth.Token
}

func (tp staticProvider) Token(context.Context) (*auth.Token, error) {
	return tp.t, nil
}
