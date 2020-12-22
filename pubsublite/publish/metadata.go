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

package publish

import (
	"fmt"
	"strconv"
	"strings"
)

// Metadata holds the results of a published message.
type Metadata struct {
	Partition int
	Offset    int64
}

func (m *Metadata) String() string {
	return fmt.Sprintf("%d:%d", m.Partition, m.Offset)
}

// ParseMetadata converts a string obtained from Metadata.String()
// back to Metadata.
func ParseMetadata(input string) (*Metadata, error) {
	parts := strings.Split(input, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("pubsublite: invalid encoded publish metadata %q", input)
	}

	partition, pErr := strconv.ParseInt(parts[0], 10, 64)
	offset, oErr := strconv.ParseInt(parts[1], 10, 64)
	if pErr != nil || oErr != nil {
		return nil, fmt.Errorf("pubsublite: invalid encoded publish metadata %q", input)
	}
	return &Metadata{Partition: int(partition), Offset: offset}, nil
}
