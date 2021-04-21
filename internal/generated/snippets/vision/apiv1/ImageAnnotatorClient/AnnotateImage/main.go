// Copyright 2021 Google LLC
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

// [START vision_generated_vision_apiv1_ImageAnnotatorClient_AnnotateImage]

package main

import (
	"context"

	vision "cloud.google.com/go/vision/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

func main() {
	ctx := context.Background()
	c, err := vision.NewImageAnnotatorClient(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	res, err := c.AnnotateImage(ctx, &pb.AnnotateImageRequest{
		Image: vision.NewImageFromURI("gs://my-bucket/my-image.png"),
		Features: []*pb.Feature{
			{Type: pb.Feature_LANDMARK_DETECTION, MaxResults: 5},
			{Type: pb.Feature_LABEL_DETECTION, MaxResults: 3},
		},
	})
	if err != nil {
		// TODO: Handle error.
	}
	// TODO: Use res.
	_ = res
}

// [END vision_generated_vision_apiv1_ImageAnnotatorClient_AnnotateImage]
