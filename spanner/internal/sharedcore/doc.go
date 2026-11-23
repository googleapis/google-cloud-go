/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package sharedcore provides the internal Go interface layer between host
// language bindings (such as Node.js cgo/N-API and Python C extensions) and
// the Cloud Spanner Go client library's transport and resumable stream
// decoding infrastructure.
//
// Internal Use Only: This package is intended only for Google Cloud client
// libraries across language repositories and is not a supported public API.
package sharedcore
