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

package debugview_test

import (
	"cloud.google.com/go/bigtable/debugview"
	"cloud.google.com/go/bigtable/internal/session"
)

// Compile-time assertion: internal/session.Client satisfies
// debugview.DebugProviders. Kept in a _test.go under debugview_test to
// avoid an import cycle — debugview cannot import internal/session
// directly (internal/session -> internal/transport, and debugview ->
// bigtable -> internal/session, so debugview importing internal/session
// would loop).
var _ debugview.DebugProviders = (session.Client)(nil)
