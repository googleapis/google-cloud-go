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

package firestore

import (
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// CloudEventDocument represents the Firebase document that triggered a Firestore
// Cloud Event (as received by a Google Cloud Function).
// It contains an old value of the Firebase document (prior to being updated)
// as well as the current value.
//
// Example:
//
//	func MyCloudFunction(ctx context.Context, e event.Event) error {
//	cloudEvent := firestore.CloudEvent{}
//	err := json.Unmarshal(e.DataEncoded, &cloudEvent)
//	if err != nil {
//		return err
//	}
//
//	c := &Client{
//		projectID:    "projectID",
//		databaseID:   "(default)",
//		readSettings: &readSettings{},
//	}
//
//	x := MyStruct{}
//	err = cloudEvent.DataTo(&x, c)
//	if err != nil {
//		return err
//	}
//
//	return nil
//	}
type CloudEventDocument struct {
	OldValue   pb.Document `firestore:"oldValue,omitempty" json:"oldValue,omitempty"`
	Value      pb.Document `firestore:"value" json:"value"`
	UpdateMask struct {
		FieldPaths []string `firestore:"fieldPaths,omitempty" json:"fieldPaths,omitempty"`
	} `firestore:"updateMask,omitempty" json:"updateMask,omitempty"`
}

// DataTo uses the updated fields in a document triggering a Firebase Cloud Event to populate p,
// which can be a pointer to a map[string]interface{} or a pointer to a struct.
// Requires a valid pointer to a Client to handle any document reference fields.
//
// Firestore field values are converted to Go values as follows:
//   - Null converts to nil.
//   - Bool converts to bool.
//   - String converts to string.
//   - Integer converts int64. When setting a struct field, any signed or unsigned
//     integer type is permitted except uint, uint64 or uintptr. Overflow is detected
//     and results in an error.
//   - Double converts to float64. When setting a struct field, float32 is permitted.
//     Overflow is detected and results in an error.
//   - Bytes is converted to []byte.
//   - Timestamp converts to time.Time.
//   - GeoPoint converts to *latlng.LatLng, where latlng is the package
//     "google.golang.org/genproto/googleapis/type/latlng".
//   - Arrays convert to []interface{}. When setting a struct field, the field
//     may be a slice or array of any type and is populated recursively.
//     Slices are resized to the incoming value's size, while arrays that are too
//     long have excess elements filled with zero values. If the array is too short,
//     excess incoming values will be dropped.
//   - Vectors convert to []float64
//   - Maps convert to map[string]interface{}. When setting a struct field,
//     maps of key type string and any value type are permitted, and are populated
//     recursively.
//   - References are converted to *firestore.DocumentRefs.
//
// Field names given by struct field tags are observed, as described in
// DocumentRef.Create.
//
// Only the fields actually present in the document are used to populate p. Other fields
// of p are left unchanged.
//
// Example:
//
//	func MyCloudFunction(ctx context.Context, e event.Event) error {
//	cloudEvent := firestore.CloudEvent{}
//	err := json.Unmarshal(e.DataEncoded, &cloudEvent)
//	if err != nil {
//		return err
//	}
//
//	c := &Client{
//		projectID:    "projectID",
//		databaseID:   "(default)",
//		readSettings: &readSettings{},
//	}
//
//	x := MyStruct{}
//	err = cloudEvent.DataTo(&x, c)
//	if err != nil {
//		return err
//	}
//
//	return nil
//	}
func (e *CloudEventDocument) DataTo(p interface{}, c *Client) error {
	// Convert the pb.Document to a DocumentSnapshot
	snapshot, err := convertProtoDocumentToSnapshot(&e.Value, c)
	if err != nil {
		return err
	}

	// Use the DocumentSnapshot's DataTo method to populate the target
	// pointer to a map[string]interface{} or a pointer to a struct
	return snapshot.DataTo(p)
}

// OldDataTo uses the old value of a document triggering a Firebase Cloud Event
// to populate p, which can be a pointer to a map[string]interface{} or a pointer to a struct.
// Requires a valid pointer to a Client to handle any document reference fields.
//
// See DataTo for a detailed usage example.
func (e *CloudEventDocument) OldDataTo(p interface{}, c *Client) error {
	// Convert the pb.Document to a DocumentSnapshot
	snapshot, err := convertProtoDocumentToSnapshot(&e.OldValue, c)
	if err != nil {
		return err
	}

	// Use the DocumentSnapshot's DataTo method to populate the target
	// pointer to a map[string]interface{} or a pointer to a struct
	return snapshot.DataTo(p)
}

// convertProtoDocumentToSnapshot converts a protobuf document to a firebase.DocumentSnapshot
func convertProtoDocumentToSnapshot(doc *pb.Document, c *Client) (*DocumentSnapshot, error) {
	// Create a DocumentRef from the document name
	docRef, err := pathToDoc(doc.Name, c)
	if err != nil {
		return nil, err
	}

	// Convert the pb.Document to a DocumentSnapshot
	snapshot, err := newDocumentSnapshot(docRef, doc, c, doc.UpdateTime)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}
