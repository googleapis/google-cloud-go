// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adapt

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Timestamp is used to parse TIMESTAMP fields coming from BigQuery APIs
type Timestamp struct {
	time.Time
}

var _ json.Unmarshaler = &Timestamp{}
var _ sql.Scanner = &Timestamp{}

// Scan assigns a value from a database driver.
func (ts *Timestamp) Scan(src any) error {
	if src == nil {
		ts.Time = time.Time{}
		return nil
	}
	switch data := src.(type) {
	case *time.Time:
		ts.Time = *data
		return nil
	case time.Time:
		ts.Time = data
		return nil
	case string:
		t, err := parseStringTimestamp(data)
		if err != nil {
			return err
		}
		ts.Time = *t
	default:
		return fmt.Errorf("invalid type `%T` for parsing timestamp, expected `string` or `time.Time`", src)
	}
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (ts *Timestamp) UnmarshalJSON(src []byte) error {
	return ts.Scan(strings.Trim(string(src), "\""))
}

func parseStringTimestamp(s string) (*time.Time, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}
	t := time.UnixMicro(i).UTC()
	return &t, nil
}
