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
