// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/outputstream"
	"cloud.google.com/go/spanner/test/cloudexecutor/executor/internal/utility"
)

func processResults(iter *spanner.RowIterator, limit int64, outcomeSender *outputstream.OutcomeSender, flowContext *ExecutionFlowContext) error {
	return nil
}

// extractTypes extracts types from given table and columns, while ignoring the child rows.
func extractTypes(table string, cols []string, metadata *utility.TableMetadataHelper) ([]*spannerpb.Type, error) {
	var typeList []*spannerpb.Type
	for _, col := range cols {
		ctype, err := metadata.GetColumnType(table, col)
		if err != nil {
			return nil, err
		}
		typeList = append(typeList, ctype)
	}
	return typeList, nil
}
