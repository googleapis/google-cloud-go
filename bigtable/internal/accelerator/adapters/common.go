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

package adapters

// ResourceKind identifies which kind of Bigtable resource a request targets.
// The adapter knows this from which V2 name field the caller populated, so
// downstream routing switches on the kind instead of re-parsing the name.
type ResourceKind int

const (
	// ResourceTable is a standard table ("projects/P/instances/I/tables/T").
	ResourceTable ResourceKind = iota
	// ResourceAuthorizedView is an authorized view
	// ("projects/P/instances/I/tables/T/authorizedViews/V").
	ResourceAuthorizedView
	// ResourceMaterializedView is a materialized view
	// ("projects/P/instances/I/materializedViews/V"). Read-only.
	ResourceMaterializedView
)

// Resource is the fully-qualified resource a request targets, tagged with its
// kind so callers route without inspecting the name string.
type Resource struct {
	Kind ResourceKind
	Name string
}

// Adapter defines a generic interface for adapting one type to another.
type Adapter[From any, To any] interface {
	Adapt(from From) (To, error)
}

// RequestAdapter represents a specialized adapter for request routing.
type RequestAdapter[From any, To any] interface {
	Adapter[From, To]
	ExtractResource(from From) (Resource, error)
}

// Default request and response adapter singletons.
var (
	DefaultReadRowRequestAdapter    = &ReadRowRequestAdapter{}
	DefaultReadRowResponseAdapter   = &ReadRowResponseAdapter{}
	DefaultMutateRowRequestAdapter  = &MutateRowRequestAdapter{}
	DefaultMutateRowResponseAdapter = &MutateRowResponseAdapter{}
)
