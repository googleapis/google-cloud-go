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
