// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law_assets/v2_query_iterator.go
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package query

import (
	"errors"
)

// RowIterator is an iterator over the results of a query.
type RowIterator struct {
}

// Next returns the next row from the results.
func (it *RowIterator) Next() (*Row, error) {
	return nil, errors.New("not implemented")
}
