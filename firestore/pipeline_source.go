// Copyright 2025 Google LLC
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

package firestore

import (
	"fmt"
	"reflect"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// PipelineSource is a factory for creating Pipeline instances.
// It is obtained by calling [Client.Pipeline()].
type PipelineSource struct {
	client *Client
}

// CollectionHints provides hints to the query planner.
type CollectionHints map[string]any

// WithForceIndex specifies an index to force the query to use.
func (ch CollectionHints) WithForceIndex(index string) CollectionHints {
	ch["force_index"] = index
	return ch
}

// WithIgnoreIndexFields specifies fields to ignore when selecting an index.
func (ch CollectionHints) WithIgnoreIndexFields(fields ...string) CollectionHints {
	ch["ignore_index_fields"] = fields
	return ch
}

func (ch CollectionHints) toProto() (map[string]*pb.Value, error) {
	if ch == nil {
		return nil, nil
	}
	optsMap := make(map[string]*pb.Value)
	for key, val := range ch {
		valPb, _, err := toProtoValue(reflect.ValueOf(val))
		if err != nil {
			return nil, fmt.Errorf("firestore: error converting option %q: %w", key, err)
		}
		optsMap[key] = valPb
	}
	return optsMap, nil
}

// CollectionOption is an option for a Collection pipeline stage.
type CollectionOption interface {
	apply(co *collectionSettings)
}

type collectionSettings struct {
	Hints CollectionHints
}

func (cs *collectionSettings) toProto() (map[string]*pb.Value, error) {
	if cs == nil {
		return nil, nil
	}
	return cs.Hints.toProto()
}

// funcCollectionOption wraps a function that modifies collectionSettings
// into an implementation of the CollectionOption interface.
type funcCollectionOption struct {
	f func(*collectionSettings)
}

func (fco *funcCollectionOption) apply(cs *collectionSettings) {
	fco.f(cs)
}

func newFuncCollectionOption(f func(*collectionSettings)) *funcCollectionOption {
	return &funcCollectionOption{
		f: f,
	}
}

// WithCollectionHints specifies hints for the query planner.
func WithCollectionHints(hints CollectionHints) CollectionOption {
	return newFuncCollectionOption(func(cs *collectionSettings) {
		cs.Hints = hints
	})
}

// Collection creates a new [Pipeline] that operates on the specified Firestore collection.
func (ps *PipelineSource) Collection(path string, opts ...CollectionOption) *Pipeline {
	cs := &collectionSettings{}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(cs)
		}
	}
	return newPipeline(ps.client, newInputStageCollection(path, cs))
}

// CollectionGroupOption is an option for a CollectionGroup pipeline stage.
type CollectionGroupOption interface {
	apply(co *collectionGroupSettings)
}

type collectionGroupSettings struct {
	Hints CollectionHints
}

func (cgs *collectionGroupSettings) toProto() (map[string]*pb.Value, error) {
	if cgs == nil {
		return nil, nil
	}
	return cgs.Hints.toProto()
}

// funcCollectionGroupOption wraps a function that modifies collectionGroupSettings
// into an implementation of the CollectionGroupOption interface.
type funcCollectionGroupOption struct {
	f func(*collectionGroupSettings)
}

func (fcgo *funcCollectionGroupOption) apply(cgs *collectionGroupSettings) {
	fcgo.f(cgs)
}

func newFuncCollectionGroupOption(f func(*collectionGroupSettings)) *funcCollectionGroupOption {
	return &funcCollectionGroupOption{
		f: f,
	}
}

// WithCollectionGroupHints specifies hints for the query planner.
func WithCollectionGroupHints(hints CollectionHints) CollectionGroupOption {
	return newFuncCollectionGroupOption(func(cgs *collectionGroupSettings) {
		cgs.Hints = hints
	})
}

// CollectionGroup creates a new [Pipeline] that operates on all documents in a group
// of collections that include the given ID, regardless of parent document.
//
// For example, consider:
// Countries/France/Cities/Paris = {population: 100}
// Countries/Canada/Cities/Montreal = {population: 90}
//
// CollectionGroup can be used to query across all "Cities" regardless of
// its parent "Countries".
func (ps *PipelineSource) CollectionGroup(collectionID string, opts ...CollectionGroupOption) *Pipeline {
	cgs := &collectionGroupSettings{}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(cgs)
		}
	}
	return newPipeline(ps.client, newInputStageCollectionGroup("", collectionID, cgs))
}

// Database creates a new [Pipeline] that operates on all documents in the Firestore database.
func (ps *PipelineSource) Database() *Pipeline {
	return newPipeline(ps.client, newInputStageDatabase())
}

// Documents creates a new [Pipeline] that operates on a specific set of Firestore documents.
func (ps *PipelineSource) Documents(refs ...*DocumentRef) *Pipeline {
	return newPipeline(ps.client, newInputStageDocuments(refs...))
}
