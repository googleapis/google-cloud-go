/*
Copyright 2018 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"math/big"
	"strings"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
)

// TODO(deklerk): move this to internal/testutil
func testEqual(a, b interface{}) bool {
	return testutil.Equal(a, b,
		cmp.AllowUnexported(TimestampBound{}, Error{}, TransactionOutcomeUnknownError{},
			Mutation{}, Row{}, Partition{}, BatchReadOnlyTransactionID{}, big.Rat{}, big.Int{}),
		cmp.FilterPath(func(path cmp.Path) bool {
			// Ignore Error.state, Error.sizeCache, and Error.unknownFields
			if strings.HasSuffix(path.GoString(), ".err.(*status.Error).state") {
				return true
			}
			if strings.HasSuffix(path.GoString(), ".err.(*status.Error).sizeCache") {
				return true
			}
			if strings.HasSuffix(path.GoString(), ".err.(*status.Error).unknownFields") {
				return true
			}
			if strings.HasSuffix(path.GoString(), ".err.(*status.Error).e") {
				return true
			}
			if strings.Contains(path.GoString(), "{*status.Error}.state") {
				return true
			}
			if strings.Contains(path.GoString(), "{*status.Error}.sizeCache") {
				return true
			}
			if strings.Contains(path.GoString(), "{*status.Error}.unknownFields") {
				return true
			}
			if strings.Contains(path.GoString(), "{*status.Error}.e") {
				return true
			}
			return false
		}, cmp.Ignore()))
}
