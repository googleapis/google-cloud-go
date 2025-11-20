// Copyright 2025 Google LLC
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

package storage

import (
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	raw "google.golang.org/api/storage/v1"
)

// ObjectContexts is a container for object contexts.
type ObjectContexts struct {
	Custom map[string]ObjectContextValue
}

// ObjectContextValue holds the value of a user-defined object context.
type ObjectContextValue struct {
	Value  string
	Delete bool
	// Read-only fields. Any updates to CreateTime and UpdateTime will be ignored.
	// These fields are handled by the server.
	CreateTime time.Time
	UpdateTime time.Time
}

// toContexts converts the raw library's ObjectContexts type to the object contexts.
func toObjectContexts(c *raw.ObjectContexts) *ObjectContexts {
	if c == nil {
		return nil
	}
	customContexts := make(map[string]ObjectContextValue)
	for k, v := range c.Custom {
		customContexts[k] = ObjectContextValue{
			Value:      v.Value,
			CreateTime: convertTime(v.CreateTime),
			UpdateTime: convertTime(v.UpdateTime),
		}
	}
	return &ObjectContexts{
		Custom: customContexts,
	}
}

// toRawObjectContexts converts the object contexts to the raw library's ObjectContexts type.
func toRawObjectContexts(c *ObjectContexts) *raw.ObjectContexts {
	if c == nil {
		return nil
	}
	customContexts := make(map[string]raw.ObjectCustomContextPayload)
	for k, v := range c.Custom {
		var payload raw.ObjectCustomContextPayload
		if v.Delete {
			// If Delete is true, populate null fields to signify deletion.
			payload.NullFields = []string{k}
		} else {
			payload = raw.ObjectCustomContextPayload{
				Value:           v.Value,
				ForceSendFields: []string{k},
			}
		}
		customContexts[k] = payload
	}
	return &raw.ObjectContexts{
		Custom: customContexts,
	}
}

func toObjectContextsFromProto(c *storagepb.ObjectContexts) *ObjectContexts {
	if c == nil {
		return nil
	}
	customContexts := make(map[string]ObjectContextValue)
	for k, v := range c.GetCustom() {
		customContexts[k] = ObjectContextValue{
			Value:      v.GetValue(),
			CreateTime: v.GetCreateTime().AsTime(),
			UpdateTime: v.GetUpdateTime().AsTime(),
		}
	}
	return &ObjectContexts{
		Custom: customContexts,
	}
}

func toProtoObjectContexts(c *ObjectContexts) *storagepb.ObjectContexts {
	if c == nil {
		return nil
	}
	customContexts := make(map[string]*storagepb.ObjectCustomContextPayload)
	for k, v := range c.Custom {
		var payload *storagepb.ObjectCustomContextPayload
		if v.Delete {
			continue
		} else {
			payload = &storagepb.ObjectCustomContextPayload{
				Value: v.Value,
			}
		}
		customContexts[k] = payload
	}
	return &storagepb.ObjectContexts{
		Custom: customContexts,
	}
}
