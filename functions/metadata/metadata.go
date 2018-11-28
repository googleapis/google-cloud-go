// Copyright 2018 Google LLC
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

package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Metadata holds Google Cloud Functions metadata.
type Metadata struct {
	// EventID is a unique ID for the event. For example: "70172329041928".
	EventID string `json:"eventId"`
	// Timestamp is the date/time this event was created.
	Timestamp time.Time `json:"timestamp"`
	// EventType is the type of the event. For example: "google.pubsub.topic.publish".
	EventType string `json:"eventType"`
	// Resource is the resource that triggered the event.
	Resource *Resource `json:"resource"`
}

// Resource holds Google Cloud Functions resource metadata.
// Resource values are dependent on the event type they're from.
type Resource struct {
	// Service is the service that triggered the event.
	Service string `json:"service"`
	// Name is the name associated with the event.
	Name string `json:"name"`
	// Type is the type of event.
	Type string `json:"type"`
}

// wrapper wraps Metadata to make nil serialization work nicely.
type wrapper struct {
	M *Metadata `json:"m,omitempty"`
}

type contextKey string

// GCFContextKey satisfies an interface to be able to use contextKey to read
// metadata from a Cloud Functions context.Context.
func (k contextKey) GCFContextKey() string {
	return string(k)
}

const metadataContextKey = contextKey("metadata")

// FromContext extracts the Metadata from the Context, if present.
func FromContext(ctx context.Context) (*Metadata, error) {
	if ctx == nil {
		return nil, errors.New("nil ctx")
	}
	b, ok := ctx.Value(metadataContextKey).(json.RawMessage)
	if !ok {
		return nil, errors.New("unable to find metadata")
	}
	w := &wrapper{}
	if err := json.Unmarshal(b, w); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %v", err)
	}
	return w.M, nil
}

// NewContext returns a new Context carrying m. NewContext is useful for
// writing tests which rely on Metadata.
func NewContext(ctx context.Context, m *Metadata) context.Context {
	b, err := json.Marshal(&wrapper{M: m})
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, metadataContextKey, json.RawMessage(b))
}
