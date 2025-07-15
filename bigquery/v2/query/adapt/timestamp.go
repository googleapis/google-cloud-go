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
	"strconv"
	"time"
)

type Timestamp struct {
	time.Time
}

var _ json.Unmarshaler = &Timestamp{}
var _ sql.Scanner = &Timestamp{}

func (ts *Timestamp) Scan(src any) error {
	if src == nil {
		return nil
	}
	t, err := ParseTimestamp(src.(string))
	if err != nil {
		return err
	}
	ts.Time = *t
	return nil
}

func (ts *Timestamp) UnmarshalJSON(src []byte) error {
	return ts.Scan(string(src))
}

func ParseTimestamp(s string) (*time.Time, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}
	t := time.UnixMicro(i).UTC()
	return &t, nil
}
