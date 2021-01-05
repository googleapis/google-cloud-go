package spanner

import (
	"context"

	"cloud.google.com/go/internal/trace"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc/codes"
)

// UpdateWithResultSet .update but returns original resultSet which contains Stats
func (t *ReadWriteTransaction) UpdateWithResultSet(ctx context.Context, stmt Statement, opts QueryOptions) (resultSet *sppb.ResultSet, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.Update")
	defer func() { trace.EndSpan(ctx, err) }()
	req, sh, err := t.prepareExecuteSQL(ctx, stmt, opts)
	if err != nil {
		return nil, err
	}
	resultSet, err = sh.getClient().ExecuteSql(contextWithOutgoingMetadata(ctx, sh.getMetadata()), req)
	if err != nil {
		return nil, ToSpannerError(err)
	}
	if resultSet.Stats == nil {
		return nil, spannerErrorf(codes.InvalidArgument, "query passed to Update: %q", stmt.SQL)
	}
	return resultSet, nil
}
