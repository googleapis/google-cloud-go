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

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

const authHeaderKey = "Authorization"

// SetAuthHeader sets the Authorization header on the provided request with a
// [cloud.google.com/go/auth.Token.Value] provided by the
// [cloud.google.com/go/auth.TokenProvider].
func SetAuthHeader(ctx context.Context, tp auth.TokenProvider, r *http.Request) error {
	t, err := tp.Token(ctx)
	if err != nil {
		return err
	}
	typ := t.Type
	if typ == "" {
		typ = internal.TokenTypeBearer
	}
	r.Header.Set(authHeaderKey, typ+" "+t.Value)
	return nil
}
