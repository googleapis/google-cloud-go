// Copyright 2026 Google LLC
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

package internal

import (
	"context"
)

// ChainInterceptors chains multiple interceptors into a single virtual RPC execution Handler.
// The first interceptor in the list is executed first, wrapping the subsequent ones.
func ChainInterceptors(interceptors ...Interceptor) Interceptor {
	return func(ctx context.Context, req interface{}, invoker Handler) (interface{}, error) {
		chain := invoker
		for i := len(interceptors) - 1; i >= 0; i-- {
			currentIdx := i
			nextHandler := chain
			chain = func(c context.Context, r interface{}) (interface{}, error) {
				return interceptors[currentIdx](c, r, nextHandler)
			}
		}
		return chain(ctx, req)
	}
}