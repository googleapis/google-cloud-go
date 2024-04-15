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

package pscompat

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/proto"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

// EventTimeAttributeKey is the key of the attribute whose value is an encoded
// Timestamp proto. The value will be used to set the event time property of a
// Pub/Sub Lite message.
const EventTimeAttributeKey = "x-goog-pubsublite-event-time-timestamp-proto"

var errInvalidMessage = errors.New("pubsublite: invalid received message")

// EncodeEventTimeAttribute encodes a timestamp in a way that it will be
// interpreted as an event time if published on a message with an attribute
// named EventTimeAttributeKey.
func EncodeEventTimeAttribute(eventTime *tspb.Timestamp) (string, error) {
	bytes, err := proto.Marshal(eventTime)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// DecodeEventTimeAttribute decodes a timestamp that was encoded with
// EncodeEventTimeAttribute.
func DecodeEventTimeAttribute(value string) (*tspb.Timestamp, error) {
	bytes, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	eventTime := &tspb.Timestamp{}
	if err := proto.Unmarshal(bytes, eventTime); err != nil {
		return nil, err
	}
	return eventTime, nil
}

// extractOrderingKey extracts the ordering key from the message for routing
// during publishing. It is the default KeyExtractorFunc implementation.
func extractOrderingKey(msg *pubsub.Message) []byte {
	if len(msg.OrderingKey) == 0 {
		return nil
	}
	return []byte(msg.OrderingKey)
}

// transformPublishedMessage is the default PublishMessageTransformerFunc
// implementation.
func transformPublishedMessage(from *pubsub.Message, to *pb.PubSubMessage, extractKey KeyExtractorFunc) error {
	to.Data = from.Data
	to.Key = extractKey(from)

	if len(from.Attributes) > 0 {
		to.Attributes = make(map[string]*pb.AttributeValues)
		for key, value := range from.Attributes {
			if key == EventTimeAttributeKey {
				eventpb, err := DecodeEventTimeAttribute(value)
				if err != nil {
					return err
				}
				to.EventTime = eventpb
			} else {
				to.Attributes[key] = &pb.AttributeValues{Values: [][]byte{[]byte(value)}}
			}
		}
	}
	return nil
}

// transformReceivedMessage is the default ReceiveMessageTransformerFunc
// implementation.
func transformReceivedMessage(from *pb.SequencedMessage, to *pubsub.Message) error {
	if from == nil || from.GetMessage() == nil {
		// This should not occur, but guard against nil.
		return errInvalidMessage
	}

	msg := from.GetMessage()

	if from.GetPublishTime() != nil {
		if err := from.GetPublishTime().CheckValid(); err != nil {
			return fmt.Errorf("%s: %s", errInvalidMessage.Error(), err)
		}
		to.PublishTime = from.GetPublishTime().AsTime()
	}
	if len(msg.GetKey()) > 0 {
		to.OrderingKey = string(msg.GetKey())
	}
	to.Data = msg.GetData()
	to.Attributes = make(map[string]string)

	if msg.EventTime != nil {
		val, err := EncodeEventTimeAttribute(msg.EventTime)
		if err != nil {
			return fmt.Errorf("%s: %s", errInvalidMessage.Error(), err)
		}
		to.Attributes[EventTimeAttributeKey] = val
	}
	for key, values := range msg.Attributes {
		if key == EventTimeAttributeKey {
			return fmt.Errorf("%s: attribute with reserved key %q exists in API message", errInvalidMessage.Error(), EventTimeAttributeKey)
		}
		if len(values.Values) > 1 {
			return fmt.Errorf("%s: cannot transform API message with multiple values for attribute with key %q", errInvalidMessage.Error(), key)
		}
		to.Attributes[key] = string(values.Values[0])
	}
	return nil
}

// MessageMetadata holds properties of a message published to the Pub/Sub Lite
// service.
type MessageMetadata struct {
	// The topic partition the message was published to.
	Partition int

	// The offset the message was assigned.
	//
	// If this MessageMetadata was returned for a publish result and publish
	// idempotence was enabled, the offset may be -1 when the message was
	// identified as a duplicate of an already successfully published message,
	// but the server did not have sufficient information to return the message's
	// offset at publish time. Messages received by subscribers will always have
	// the correct offset.
	Offset int64
}

func (m *MessageMetadata) String() string {
	return fmt.Sprintf("%d:%d", m.Partition, m.Offset)
}

// ParseMessageMetadata creates MessageMetadata from the ID string of a
// pubsub.PublishResult returned by PublisherClient or pubsub.Message.ID
// received from SubscriberClient.
func ParseMessageMetadata(id string) (*MessageMetadata, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("pubsublite: invalid encoded message metadata %q", id)
	}
	partition, pErr := strconv.Atoi(parts[0])
	offset, oErr := strconv.ParseInt(parts[1], 10, 64)
	if pErr != nil || oErr != nil {
		return nil, fmt.Errorf("pubsublite: invalid encoded message metadata %q", id)
	}
	return &MessageMetadata{Partition: partition, Offset: offset}, nil
}
