// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package longrunning

import "errors"

// A custom method on the LRO autogen to propagate the API interface version to the autogen
// helper client.  This is used internally by Google client code and not intended for general
// use.
func (c *OperationsClient) SetSourceAPIVersion(versionID string) error {
	key := "x-goog-api-version"

	ic := c.internalClient
	if gc, ok := ic.(*operationsGRPCClient); ok {
		gc.xGoogHeaders = append(gc.xGoogHeaders, key, versionID)
		return nil
	}
	if rc, ok := ic.(*operationsRESTClient); ok {
		rc.xGoogHeaders = append(rc.xGoogHeaders, key, versionID)
		return nil
	}
	return errors.New("could not set API version")
}
