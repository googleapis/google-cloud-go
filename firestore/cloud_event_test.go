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
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/genproto/googleapis/type/latlng"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestFirestoreCloudEvent_DataTo_DocumentPaths(t *testing.T) {
	client := &Client{
		projectID:  "projectID",
		databaseID: "(default)",
	}

	tests := []struct {
		name     string
		docName  string
		wantErr  bool
		wantPath string
	}{
		{
			name:     "valid simple path",
			docName:  "projects/projectID/databases/(default)/documents/collection/document",
			wantErr:  false,
			wantPath: "projects/projectID/databases/(default)/documents/collection/document",
		},
		{
			name:     "valid nested path",
			docName:  "projects/projectID/databases/(default)/documents/collection/document/subcollection/subdocument",
			wantErr:  false,
			wantPath: "projects/projectID/databases/(default)/documents/collection/document/subcollection/subdocument",
		},
		{
			name:    "empty path",
			docName: "",
			wantErr: true,
		},
		{
			name:    "invalid path - missing database",
			docName: "projects/projectID/databases",
			wantErr: true,
		},
		{
			name:    "invalid path - wrong format",
			docName: "invalid/path/format",
			wantErr: true,
		},
		{
			name:    "invalid path - collection path without document",
			docName: "projects/projectID/databases/(default)/documents/collection",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudEvent := &CloudEventDocument{
				Value: pb.Document{
					Name:       tt.docName,
					CreateTime: testTimestampProto,
					UpdateTime: testTimestampProto,
					Fields: map[string]*pb.Value{
						"stringData": {
							ValueType: &pb.Value_StringValue{
								StringValue: "test",
							},
						},
					},
				},
			}

			var result map[string]interface{}
			err := cloudEvent.DataTo(&result, client)
			if (err != nil) != tt.wantErr {
				t.Errorf("DataTo() with path %q error = %v, wantErr %v", tt.docName, err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the data conversion worked
				if result["stringData"] != "test" {
					t.Errorf("DataTo() result[stringData] = %v, want test", result["stringData"])
				}
			}
		})
	}
}

func TestFirestoreCloudEvent_DataTo(t *testing.T) {
	cloudEvent := &CloudEventDocument{
		Value: pb.Document{
			Name:       testCloudEventDoc.Name,
			CreateTime: testCloudEventDoc.CreateTime,
			UpdateTime: testCloudEventDoc.UpdateTime,
			Fields:     testCloudEventDoc.Fields,
		},
	}

	t.Run("DataTo tagged struct", func(t *testing.T) {
		var result TestTaggedStruct
		err := cloudEvent.DataTo(&result, testClient)
		if err != nil {
			t.Fatalf("DataTo() error = %v", err)
		}

		if !testEqual(result, expectedTestTaggedStruct) {
			t.Errorf("cloudEvent.DataTo(TestTaggedStruct) does not match expected value")
			t.Logf("Diff: %s", testDiff(result, expectedTestTaggedStruct))
		}
	})

	t.Run("DataTo map", func(t *testing.T) {
		var result map[string]interface{}
		err := cloudEvent.DataTo(&result, testClient)
		if err != nil {
			t.Fatalf("DataTo() error = %v", err)
		}

		if !testEqual(result, expectedTestMap) {
			t.Errorf("cloudEvent.DataTo(TestTaggedStruct) does not match expected value")
			t.Logf("Diff: %s", testDiff(result, expectedTestTaggedStruct))
		}
	})

	t.Run("DataTo with empty document name", func(t *testing.T) {
		emptyNameEvent := &CloudEventDocument{
			Value: pb.Document{
				CreateTime: testTimestampProto,
				UpdateTime: testTimestampProto,
				Fields:     testCloudEventDoc.Fields,
			},
		}

		var result map[string]interface{}
		err := emptyNameEvent.DataTo(&result, testClient)
		if err == nil {
			t.Error("DataTo() with empty name should return error")
		}
	})

	t.Run("DataTo with invalid document name", func(t *testing.T) {
		invalidEvent := &CloudEventDocument{
			Value: pb.Document{
				Name:       "invalid/path/format",
				CreateTime: testTimestampProto,
				UpdateTime: testTimestampProto,
				Fields:     testCloudEventDoc.Fields,
			},
		}

		var result map[string]interface{}
		err := invalidEvent.DataTo(&result, testClient)
		if err == nil {
			t.Error("DataTo() with invalid document name should return error")
		}
	})
}

func TestFirestoreCloudEvent_DataTo_ErrorCases(t *testing.T) {
	client := &Client{
		projectID:  "projectID",
		databaseID: "(default)",
	}

	t.Run("nil client", func(t *testing.T) {
		cloudEvent := &CloudEventDocument{
			Value: pb.Document{
				Name:       "projects/projectID/databases/(default)/documents/collection/document",
				CreateTime: testTimestampProto,
				UpdateTime: testTimestampProto,
				Fields:     testCloudEventDoc.Fields,
			},
		}

		// Calling FirestoreCloudEvent.DataTo With a nil client will panic
		defer func() {
			// Just recover from any panic
			recover()
		}()

		var result map[string]interface{}
		cloudEvent.DataTo(&result, nil)
	})

	t.Run("nil pointer", func(t *testing.T) {
		cloudEvent := &CloudEventDocument{
			Value: pb.Document{
				Name:       "projects/projectID/databases/(default)/documents/collection/document",
				CreateTime: testTimestampProto,
				UpdateTime: testTimestampProto,
				Fields:     testCloudEventDoc.Fields,
			},
		}

		err := cloudEvent.DataTo(nil, client)
		if err == nil {
			t.Error("DataTo() with nil pointer should return error")
		}
	})

	t.Run("non-pointer argument", func(t *testing.T) {
		cloudEvent := &CloudEventDocument{
			Value: pb.Document{
				Name:       "projects/projectID/databases/(default)/documents/collection/document",
				CreateTime: testTimestampProto,
				UpdateTime: testTimestampProto,
				Fields:     testCloudEventDoc.Fields,
			},
		}

		var result map[string]interface{}
		err := cloudEvent.DataTo(result, client) // Not a pointer
		if err == nil {
			t.Error("DataTo() with non-pointer should return error")
		}
	})
}

// Test struct with tags that do not match field names
type TestTaggedStruct struct {
	Time      time.Time              `firestore:"timeData"`
	String    string                 `firestore:"stringData"`
	Bool      bool                   `firestore:"boolData"`
	Int       int64                  `firestore:"intData"`
	Double    float64                `firestore:"doubleData"`
	Bytes     []byte                 `firestore:"bytesData"`
	Nil       interface{}            `firestore:"nilData"`
	GeoPoint  *latlng.LatLng         `firestore:"geoPointData"`
	Ref       *DocumentRef           `firestore:"referenceData"`
	NestedMap map[string]interface{} `firestore:"nestedMapData"`
}

var testTimestamp = time.Date(2025, 4, 14, 1, 2, 3, 0, time.UTC)
var testTimestampProto = &tspb.Timestamp{Seconds: testTimestamp.Unix(), Nanos: int32(testTimestamp.Nanosecond())}
var testLatLng = &latlng.LatLng{Latitude: 50.933986479906764, Longitude: 5.357702612564693}

// Standardized test document reference
var testDocumentRef = &DocumentRef{
	Parent:       testCollectionRef,
	Path:         "projects/projectID/databases/(default)/documents/collection/document",
	shortPath:    "collection/document",
	ID:           "document",
	readSettings: &readSettings{},
}

var testCollectionRef = &CollectionRef{
	c:            testClient,
	parentPath:   "projects/projectID/databases/(default)/documents",
	selfPath:     "collection",
	Path:         "projects/projectID/databases/(default)/documents/collection",
	ID:           "collection",
	Parent:       nil, // This would be the parent document, but it's nil for top-level collections
	readSettings: &readSettings{},
	Query: Query{
		c:            testClient,
		path:         "projects/projectID/databases/(default)/documents/collection",
		parentPath:   "projects/projectID/databases/(default)/documents",
		collectionID: "collection",
		selection:    nil,
		filters:      nil,
	},
}

// Expected result when calling DataTo(&TestTaggedStruct{})
var expectedTestTaggedStruct = TestTaggedStruct{
	Time:      testTimestamp,
	String:    "Hello World",
	Bool:      true,
	Int:       987654321,
	Double:    987.123456,
	Bytes:     []byte("Hello World"),
	Nil:       nil,
	GeoPoint:  testLatLng,
	Ref:       testDocumentRef,
	NestedMap: testNestedMap,
}

// Expected result when calling DataTo(map[string]interface{})
var expectedTestMap = map[string]interface{}{
	"timeData":      testTimestamp,
	"stringData":    "Hello World",
	"boolData":      true,
	"intData":       int64(987654321),
	"doubleData":    987.123456,
	"bytesData":     []byte("Hello World"),
	"nilData":       nil,
	"geoPointData":  testLatLng,
	"referenceData": testDocumentRef,
	"nestedMapData": testNestedMap,
}

// Complex nested data structure containing a mix of all possible data types to test recursion
var testNestedMap = map[string]interface{}{
	"nestedArrayData": []interface{}{
		map[string]interface{}{
			"timeData":      testTimestamp,
			"stringData":    "Hello World",
			"uuidData":      "1f117a40-8bdb-4e8a-8f24-1622fea695b1",
			"boolData":      true,
			"intData":       int64(987654321),
			"doubleData":    987.123456,
			"bytesData":     []byte("Hello World"),
			"nilData":       nil,
			"referenceData": testDocumentRef,
			"geoPointData":  testLatLng,
		},
		map[string]interface{}{
			"subNestedArrayData": []interface{}{
				testTimestamp,
				"Hello World",
				"1f117a40-8bdb-4e8a-8f24-1622fea695b1",
				true,
				int64(987654321),
				987.123456,
				[]byte("Hello World"),
				nil,
				testDocumentRef,
				testLatLng,
			},
		},
		testTimestamp,
		"Hello World",
		"1f117a40-8bdb-4e8a-8f24-1622fea695b1",
		true,
		int64(987654321),
		987.123456,
		[]byte("Hello World"),
		nil,
		testDocumentRef,
		testLatLng,
	},
}

// Mock Firebase Cloud Event document payload
var testCloudEventDoc = pb.Document{
	Name:       "projects/projectID/databases/(default)/documents/collection/document",
	CreateTime: testTimestampProto,
	UpdateTime: testTimestampProto,
	Fields: map[string]*pb.Value{
		"timeData": {
			ValueType: &pb.Value_TimestampValue{
				TimestampValue: testTimestampProto,
			},
		},
		"stringData": {
			ValueType: &pb.Value_StringValue{
				StringValue: "Hello World",
			},
		},
		"boolData": {
			ValueType: &pb.Value_BooleanValue{
				BooleanValue: true,
			},
		},
		"intData": {
			ValueType: &pb.Value_IntegerValue{
				IntegerValue: 987654321,
			},
		},
		"doubleData": {
			ValueType: &pb.Value_DoubleValue{
				DoubleValue: 987.123456,
			},
		},
		"bytesData": {
			ValueType: &pb.Value_BytesValue{
				BytesValue: []byte("Hello World"),
			},
		},
		"nilData": {
			ValueType: &pb.Value_NullValue{},
		},
		"geoPointData": {
			ValueType: &pb.Value_GeoPointValue{
				GeoPointValue: testLatLng,
			},
		},
		"referenceData": {
			ValueType: &pb.Value_ReferenceValue{
				ReferenceValue: "projects/projectID/databases/(default)/documents/collection/document",
			},
		},
		"nestedMapData": {
			ValueType: &pb.Value_MapValue{
				MapValue: &pb.MapValue{
					Fields: map[string]*pb.Value{
						"nestedArrayData": {
							ValueType: &pb.Value_ArrayValue{
								ArrayValue: &pb.ArrayValue{
									Values: []*pb.Value{
										{
											ValueType: &pb.Value_MapValue{
												MapValue: &pb.MapValue{
													Fields: map[string]*pb.Value{
														"timeData": {
															ValueType: &pb.Value_TimestampValue{
																TimestampValue: testTimestampProto,
															},
														},
														"stringData": {
															ValueType: &pb.Value_StringValue{
																StringValue: "Hello World",
															},
														},
														"uuidData": {
															ValueType: &pb.Value_StringValue{
																StringValue: "1f117a40-8bdb-4e8a-8f24-1622fea695b1",
															},
														},
														"boolData": {
															ValueType: &pb.Value_BooleanValue{
																BooleanValue: true,
															},
														},
														"intData": {
															ValueType: &pb.Value_IntegerValue{
																IntegerValue: 987654321,
															},
														},
														"doubleData": {
															ValueType: &pb.Value_DoubleValue{
																DoubleValue: 987.123456,
															},
														},
														"bytesData": {
															ValueType: &pb.Value_BytesValue{
																BytesValue: []byte("Hello World"),
															},
														},
														"nilData": {
															ValueType: &pb.Value_NullValue{},
														},
														"referenceData": {
															ValueType: &pb.Value_ReferenceValue{
																ReferenceValue: "projects/projectID/databases/(default)/documents/collection/document",
															},
														},
														"geoPointData": {
															ValueType: &pb.Value_GeoPointValue{
																GeoPointValue: testLatLng,
															},
														},
													},
												},
											},
										},
										{
											ValueType: &pb.Value_MapValue{
												MapValue: &pb.MapValue{
													Fields: map[string]*pb.Value{
														"subNestedArrayData": {
															ValueType: &pb.Value_ArrayValue{
																ArrayValue: &pb.ArrayValue{
																	Values: []*pb.Value{
																		{
																			ValueType: &pb.Value_TimestampValue{
																				TimestampValue: testTimestampProto,
																			},
																		},
																		{
																			ValueType: &pb.Value_StringValue{
																				StringValue: "Hello World",
																			},
																		},
																		{
																			ValueType: &pb.Value_StringValue{
																				StringValue: "1f117a40-8bdb-4e8a-8f24-1622fea695b1",
																			},
																		},
																		{
																			ValueType: &pb.Value_BooleanValue{
																				BooleanValue: true,
																			},
																		},
																		{
																			ValueType: &pb.Value_IntegerValue{
																				IntegerValue: 987654321,
																			},
																		},
																		{
																			ValueType: &pb.Value_DoubleValue{
																				DoubleValue: 987.123456,
																			},
																		},
																		{
																			ValueType: &pb.Value_BytesValue{
																				BytesValue: []byte("Hello World"),
																			},
																		},
																		{
																			ValueType: &pb.Value_NullValue{},
																		},
																		{
																			ValueType: &pb.Value_ReferenceValue{
																				ReferenceValue: "projects/projectID/databases/(default)/documents/collection/document",
																			},
																		},
																		{
																			ValueType: &pb.Value_GeoPointValue{
																				GeoPointValue: testLatLng,
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											ValueType: &pb.Value_TimestampValue{
												TimestampValue: testTimestampProto,
											},
										},
										{
											ValueType: &pb.Value_StringValue{
												StringValue: "Hello World",
											},
										},
										{
											ValueType: &pb.Value_StringValue{
												StringValue: "1f117a40-8bdb-4e8a-8f24-1622fea695b1",
											},
										},
										{
											ValueType: &pb.Value_BooleanValue{
												BooleanValue: true,
											},
										},
										{
											ValueType: &pb.Value_IntegerValue{
												IntegerValue: 987654321,
											},
										},
										{
											ValueType: &pb.Value_DoubleValue{
												DoubleValue: 987.123456,
											},
										},
										{
											ValueType: &pb.Value_BytesValue{
												BytesValue: []byte("Hello World"),
											},
										},
										{
											ValueType: &pb.Value_NullValue{},
										},
										{
											ValueType: &pb.Value_ReferenceValue{
												ReferenceValue: "projects/projectID/databases/(default)/documents/collection/document",
											},
										},
										{
											ValueType: &pb.Value_GeoPointValue{
												GeoPointValue: testLatLng,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}
