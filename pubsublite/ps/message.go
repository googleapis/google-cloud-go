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

package ps

import (
	"encoding/base64"
	"errors"
	"fmt"

	"cloud.google.com/go/pubsub"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// Message transforms and event timestamp encoding mirrors the Java client
// library implementation:
// https://github.com/googleapis/java-pubsublite/blob/master/google-cloud-pubsublite/src/main/java/com/google/cloud/pubsublite/cloudpubsub/MessageTransforms.java
const eventTimestampAttributeKey = "x-goog-pubsublite-event-time-timestamp-proto"

var errInvalidMessage = errors.New("pubsublite: invalid received message")

// Encodes a timestamp in a way that it will be interpreted as an event time if
// published on a message with an attribute named eventTimestampAttributeKey.
func encodeEventTimestamp(eventTime *tspb.Timestamp) (string, error) {
	bytes, err := proto.Marshal(eventTime)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// Decodes a timestamp encoded with encodeEventTimestamp.
func decodeEventTimestamp(value string) (*tspb.Timestamp, error) {
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
			if key == eventTimestampAttributeKey {
				eventpb, err := decodeEventTimestamp(value)
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

	var err error
	msg := from.GetMessage()

	if from.GetPublishTime() != nil {
		if to.PublishTime, err = ptypes.Timestamp(from.GetPublishTime()); err != nil {
			return fmt.Errorf("%s: %s", errInvalidMessage.Error(), err)
		}
	}
	if from.GetCursor() != nil {
		to.ID = fmt.Sprintf("%d", from.GetCursor().GetOffset())
	}
	if len(msg.GetKey()) > 0 {
		to.OrderingKey = string(msg.GetKey())
	}
	to.Data = msg.GetData()
	to.Attributes = make(map[string]string)

	if msg.EventTime != nil {
		val, err := encodeEventTimestamp(msg.EventTime)
		if err != nil {
			return fmt.Errorf("%s: %s", errInvalidMessage.Error(), err)
		}
		to.Attributes[eventTimestampAttributeKey] = val
	}
	for key, values := range msg.Attributes {
		if key == eventTimestampAttributeKey {
			return fmt.Errorf("%s: attribute with reserved key %q exists in API message", errInvalidMessage.Error(), eventTimestampAttributeKey)
		}
		if len(values.Values) > 1 {
			return fmt.Errorf("%s: cannot transform API message with multiple values for attribute with key %q", errInvalidMessage.Error(), key)
		}
		to.Attributes[key] = string(values.Values[0])
	}
	return nil
}
