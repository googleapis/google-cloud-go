// Copyright 2024 Google LLC
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

package datastore

import (
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// datastoreConfig contains the Datastore client option configuration that can be
// set through datastoreClientOptions.
type datastoreConfig struct {
	ignoreFieldMismatchErrors bool
}

// newDatastoreConfig generates a new datastoreConfig with all the given
// datastoreClientOptions applied.
func newDatastoreConfig(opts ...option.ClientOption) datastoreConfig {
	var conf datastoreConfig
	for _, opt := range opts {
		if datastoreOpt, ok := opt.(datastoreClientOption); ok {
			datastoreOpt.applyDatastoreOpt(&conf)
		}
	}
	return conf
}

// A datastoreClientOption is an option for a Google Datastore client.
type datastoreClientOption interface {
	option.ClientOption
	applyDatastoreOpt(*datastoreConfig)
}

// WithIgnoreFieldMismatch allows ignoring ErrFieldMismatch error while
// reading or querying data.
// WARNING: Ignoring ErrFieldMismatch can cause data loss while writing
// back to Datastore. E.g.
// if entity written to Datastore is {X: 1, Y:2} and it is read into
// type NewStruct struct{X int}, then {X:1} is returned.
// Now, if this is written back to Datastore, there will be no Y field
// left for this entity in Datastore
func WithIgnoreFieldMismatch() option.ClientOption {
	return &withIgnoreFieldMismatch{ignoreFieldMismatchErrors: true}
}

type withIgnoreFieldMismatch struct {
	internaloption.EmbeddableAdapter
	ignoreFieldMismatchErrors bool
}

func (w *withIgnoreFieldMismatch) applyDatastoreOpt(c *datastoreConfig) {
	c.ignoreFieldMismatchErrors = true
}
