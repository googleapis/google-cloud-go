// Copyright 2020 Google LLC
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

package wire

import (
	"strconv"
	"strings"
)

type version struct {
	Major int64
	Minor int64
}

// parseVersion attempts to parse the pubsublite library version in the format:
// "1.2.3".
func parseVersion(value string) (v version, ok bool) {
	components := strings.Split(value, ".")
	if len(components) >= 2 {
		var err error
		if v.Major, err = strconv.ParseInt(components[0], 10, 32); err != nil {
			return
		}
		if v.Minor, err = strconv.ParseInt(components[1], 10, 32); err != nil {
			return
		}
		ok = true
	}
	return
}
