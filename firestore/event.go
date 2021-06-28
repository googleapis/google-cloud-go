// Copyright 2017 Google LLC
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

package firestore

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/functions/metadata"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// FirestoreEvent is the payload of a Firestore event.
type FirestoreEvent struct {
	OldValue   FirestoreValue `json:"oldValue"`
	Value      FirestoreValue `json:"value"`
	UpdateMask struct {
		FieldPaths []string `json:"fieldPaths"`
	} `json:"updateMask"`
}

// FirestoreValue holds Firestore fields.
type FirestoreValue struct {
	CreateTime time.Time   `json:"createTime"`
	Fields     interface{} `json:"fields"`
	Name       string      `json:"name"`
	UpdateTime time.Time   `json:"updateTime"`
}

/*
NewSnapshotFromValue converts a FirestoreValue to a DocumentSnapshot.

Example:
	var client *firestore.Client

	type Model = struct {
		Something string `firestore:"something"`
	}

	func OnWrite(ctx context.Context, event firestore.FirestoreEvent) error {
		doc, err := client.NewSnapshotFromValue(ctx, event.Value)
		if err != nil {
			return err
		}

		if !doc.Exists() {
			fmt.Printf("doc %s removed", doc.Ref.ID)
			return nil
		}

		model := &Model{}
		err = doc.DataTo(model)
		if err != nil {
			return err
		}

		fmt.Println(model.Something)
		return nil
	}
*/
func (c *Client) NewSnapshotFromValue(ctx context.Context, value FirestoreValue) (*DocumentSnapshot, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	proto := &pb.Document{}
	err = protojson.Unmarshal(data, proto)
	if err != nil {
		return nil, err
	}

	name := proto.Name
	if name == "" {
		meta, err := metadata.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		name = meta.Resource.RawPath
	}

	docRef, err := pathToDoc(name, c)
	if err != nil {
		return nil, err
	}

	readTime := proto.UpdateTime

	if proto.Fields == nil {
		proto = nil
	}

	return newDocumentSnapshot(docRef, proto, c, readTime)
}
