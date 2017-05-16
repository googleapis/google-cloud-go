// Copyright 2017, Google Inc. All rights reserved.
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

package vision

import (
	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	pb "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

// AnnotateImage runs image detection and annotation for a single image.
func (c *ImageAnnotatorClient) AnnotateImage(ctx context.Context, req *pb.AnnotateImageRequest, opts ...gax.CallOption) (*pb.AnnotateImageResponse, error) {
	res, err := c.BatchAnnotateImages(ctx, &pb.BatchAnnotateImagesRequest{Requests: []*pb.AnnotateImageRequest{req}})
	if err != nil {
		return nil, err
	}
	return res.Responses[0], nil
}
