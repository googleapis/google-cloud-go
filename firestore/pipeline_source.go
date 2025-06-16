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

// PipelineSource is a factory for creating Pipeline instances.
// It is obtained by calling [Client.Pipeline()].
type PipelineSource struct {
	client *Client
}

// Collection creates a new [Pipeline] that operates on the specified Firestore collection.
func (ps *PipelineSource) Collection(path string) *Pipeline {
	return newPipeline(ps.client, newInputStageCollection(path))
}

// CollectionGroup creates a new [Pipeline] that operates on all documents in a group
// of collections that include the given ID, regardless of parent document.
//
// For example, consider:
// France/Cities/Paris = {population: 100}
// Canada/Cities/Montreal = {population: 90}
//
// CollectionGroup can be used to query across all "Cities" regardless of
// its parent "Countries".
func (ps *PipelineSource) CollectionGroup(collectionID string) *Pipeline {
	return newPipeline(ps.client, newInputStageCollectionGroup("", collectionID))
}

// CollectionGroupWithAncestor creates a new [Pipeline] that operates on all documents in a group
// of collections that include the given ID, that are underneath a given document.
//
// For example, consider:
// /continents/Europe/Germany/Cities/Paris = {population: 100}
// /continents/Europe/France/Cities/Paris = {population: 100}
// /continents/NorthAmerica/Canada/Cities/Montreal = {population: 90}
//
// CollectionGroupWithAncestor can be used to query across all "Cities" in "/continents/Europe".
func (ps *PipelineSource) CollectionGroupWithAncestor(ancestor, collectionID string) *Pipeline {
	return newPipeline(ps.client, newInputStageCollectionGroup(ancestor, collectionID))
}

// Database creates a new [Pipeline] that operates on all documents in the Firestore database.
func (ps *PipelineSource) Database() *Pipeline {
	return newPipeline(ps.client, newInputStageDatabase())
}
