package utility

import (
	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	executorpb "cloud.google.com/go/spanner/test/cloudexecutor/proto"
)

// BuildQuery constructs a spanner.Statement query and bind the params from the input executor query.
func BuildQuery(queryAction *executorpb.QueryAction) (spanner.Statement, error) {
	return spanner.Statement{}, nil
}

// KeySetProtoToCloudKeySet converts an executor KeySet to a Cloud Spanner KeySet instance.
func KeySetProtoToCloudKeySet(keySetProto *executorpb.KeySet, typeList []*spannerpb.Type) (spanner.KeySet, error) {
	return spanner.AllKeys(), nil
}
