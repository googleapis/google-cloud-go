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

// Package publish contains utilities related to publishing messages.
package publish

import (
	"fmt"
	"strconv"
	"strings"
)

// Metadata holds the result of publishing a message to the Pub/Sub Lite
// service.
type Metadata struct {
	// The topic partition the message was published to.
	Partition int

	// The offset the message was assigned.
	Offset int64
}

func (m *Metadata) String() string {
	return fmt.Sprintf("%d:%d", m.Partition, m.Offset)
}

// ParseMetadata converts the ID string of a pubsub.PublishResult to Metadata.
//
// Example:
//   result := publisher.Publish(ctx, &pubsub.Message{Data: []byte("payload")})
//   id, err := result.Get(ctx)
//   if err != nil {
//     // TODO: Handle error.
//   }
//   metadata, err := publish.ParseMetadata(id)
//   if err != nil {
//     // TODO: Handle error.
//   }
func ParseMetadata(id string) (*Metadata, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("pubsublite: invalid encoded publish metadata %q", id)
	}

	partition, pErr := strconv.ParseInt(parts[0], 10, 64)
	offset, oErr := strconv.ParseInt(parts[1], 10, 64)
	if pErr != nil || oErr != nil {
		return nil, fmt.Errorf("pubsublite: invalid encoded publish metadata %q", id)
	}
	return &Metadata{Partition: int(partition), Offset: offset}, nil
}
