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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type Bytes []byte

var _ json.Unmarshaler = &Bytes{}
var _ sql.Scanner = &Bytes{}

func (b *Bytes) Scan(src any) error {
	if src == nil {
		*b = nil
		return nil
	}
	data, ok := src.(string)
	if !ok {
		return fmt.Errorf("invalid type `%T` for parsing as `Bytes`, expected `string`", src)
	}
	value, err := parseBytes(data)
	if err != nil {
		return err
	}
	*b = value
	return nil
}

func (b *Bytes) UnmarshalJSON(src []byte) error {
	return b.Scan(strings.Trim(string(src), "\""))
}

func parseBytes(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}
