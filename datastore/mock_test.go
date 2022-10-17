// Copyright 2022 Google LLC
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

package datastore

// Simple mock server for validating service requests.
//
// This mockServer follows the paradigm set here:
// https://github.com/googleapis/google-cloud-go/blob/main/firestore/mock_test.go
//
// You must add new methods to this server when testing additional

import (
	"github.com/golang/protobuf/proto"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
)

type mockServer struct {
	pb.DatastoreServer

	Addr     string
	reqItems []reqItem
	resps    []interface{}
}

type reqItem struct {
	wantReq proto.Message
	adjust  func(gotReq proto.Message)
}
