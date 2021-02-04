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

// Package types contains shared types for pubsublite.
package types

import (
	"fmt"
	"strconv"
	"strings"
)

// MessageMetadata holds properties of a message published to the Pub/Sub Lite
// service.
type MessageMetadata struct {
	// The topic partition the message was published to.
	Partition int

	// The offset the message was assigned.
	Offset int64
}

func (m *MessageMetadata) String() string {
	return fmt.Sprintf("%d:%d", m.Partition, m.Offset)
}

// ParseMessageMetadata creates MessageMetadata from the ID string of a
// pubsub.PublishResult returned by pscompat.PublisherClient, or
// pubsub.Message.ID received from pscompat.SubscriberClient.
func ParseMessageMetadata(id string) (*MessageMetadata, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("pubsublite: invalid encoded message metadata %q", id)
	}

	partition, pErr := strconv.ParseInt(parts[0], 10, 64)
	offset, oErr := strconv.ParseInt(parts[1], 10, 64)
	if pErr != nil || oErr != nil {
		return nil, fmt.Errorf("pubsublite: invalid encoded message metadata %q", id)
	}
	return &MessageMetadata{Partition: int(partition), Offset: offset}, nil
}
