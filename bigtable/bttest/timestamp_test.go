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
package bttest

import (
	"math"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
)

func TestTimestampConversion(t *testing.T) {
	// 1. Test a Timestamp converting to Time.
	var ts1 bigtable.Timestamp = 1583863200000000
	t1 := ts1.Time().UTC()
	want1 := time.Date(2020, time.March, 10, 18, 0, 0, 0, time.UTC)

	if !want1.Equal(t1) {
		t.Errorf("Mismatched time got %v wanted %v", t1, want1)
	}
	// 2. Test a reversed Timestamp converting to Time.
	reverse := math.MaxInt64 - ts1

	got2 := reverse.Time().UTC()
	want2 := time.Date(294196, time.October, 31, 10, 0, 54, 775807000, time.UTC)

	if !want2.Equal(got2) {
		t.Errorf("Mismatched time got %v wanted %v", got2, want2)
	}

	// 3. Test a Time converted to Timestamp then converted back to Time.
	t2 := time.Date(2016, time.October, 3, 14, 7, 7, 0, time.UTC)
	ts2 := bigtable.Timestamp(t2.UnixNano() / 1000)

	got3 := ts2.Time().UTC()
	want3 := time.Date(2016, time.October, 3, 14, 7, 7, 0, time.UTC)
	if !want3.Equal(got3) {
		t.Errorf("Mismatched time got %v wanted %v", got3, want3)
	}
}
