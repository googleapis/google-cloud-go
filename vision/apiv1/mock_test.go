// Copyright 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package vision

import (
	visionpb "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

import (
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockImageAnnotator struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ visionpb.ImageAnnotatorServer = &mockImageAnnotator{}

func (s *mockImageAnnotator) BatchAnnotateImages(_ context.Context, req *visionpb.BatchAnnotateImagesRequest) (*visionpb.BatchAnnotateImagesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*visionpb.BatchAnnotateImagesResponse), nil
}
