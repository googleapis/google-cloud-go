// Copyright 2020 Google LLC
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

package bigquery

import (
	"testing"

	"cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/internal/testutil"
	bq "google.golang.org/api/bigquery/v2"
)

func TestPolicyConversions(t *testing.T) {

	for _, test := range []struct {
		bq  *bq.Policy
		iam *iampb.Policy
	}{
		{&bq.Policy{}, &iampb.Policy{}},
		{
			&bq.Policy{
				Bindings: []*bq.Binding{
					{
						Role:    "foo",
						Members: []string{"a", "b", "c"},
					},
				},
				Etag:    "etag",
				Version: 1,
			},
			&iampb.Policy{
				Bindings: []*iampb.Binding{
					{
						Role:    "foo",
						Members: []string{"a", "b", "c"},
					},
				},
				Etag:    []byte("etag"),
				Version: 1,
			},
		},
	} {
		gotIAM := iamFromBigQueryPolicy(test.bq)
		if diff := testutil.Diff(gotIAM, test.iam); diff != "" {
			t.Errorf("%+v:\n, -got, +want:\n%s", test.iam, diff)
		}

		gotBQ := iamToBigQueryPolicy(test.iam)
		if diff := testutil.Diff(gotBQ, test.bq); diff != "" {
			t.Errorf("%+v:\n, -got, +want:\n%s", test.iam, diff)
		}
	}
}
