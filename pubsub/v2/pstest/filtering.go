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

package pstest

import (
	filter "cloud.google.com/go/pubsub/v2/pstest/internal"
)

const (
	attributesStr = "attributes"
	hasPrefixStr  = "hasPrefix"
)

// ValidateFilter validates if the filter string is parsable.
func ValidateFilter(f string) error {
	_, err := parseFilter(f)
	return err
}

// parseFilter validates a filter string and returns an AST node.
func parseFilter(filterStr string) (filter.ASTNode, error) {
	return filter.ParseFilter(filterStr)
}
