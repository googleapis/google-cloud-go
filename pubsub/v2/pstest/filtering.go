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
	"cloud.google.com/go/internal/filter"
)

const (
	attributesStr = "attributes"
	hasPrefixStr  = "hasPrefix"
)

// ValidateFilter validates if the filter string is parsable.
func ValidateFilter(filter string) error {
	_, err := parseFilter(filter)
	return err
}

// parseFilter validates a filter string and returns an AST node.
func parseFilter(filterStr string) (filter.ASTNode, error) {
	return filter.ParseFilter(filterStr)
}

// filterByAttrs efficiently deletes unmatched items from the map.
func filterByAttrs[T map[K]U, U any, K comparable](items T, f filter.ASTNode, getAttrs func(U) map[string]string) {
	if f == nil {
		return
	}
	for key, item := range items {
		attrs := getAttrs(item)
		if !filter.Evaluate(f, attrs) {
			delete(items, key)
		}
	}
}





