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

// Collection returns all documents from the entire collection.
func (ps *PipelineSource) Collection(path string) *Pipeline {
	return newPipeline(ps.client, newInputStageCollection(path))
}
