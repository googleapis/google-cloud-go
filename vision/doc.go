// Copyright 2016 Google Inc. All Rights Reserved.
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

/*
Package vision provides a client for the Google Cloud Vision API.

Google Cloud Vision allows easy integration of vision detection features
into developer applications, including image labeling, face and landmark
detection, optical character recognition (OCR), and tagging of explicit
content. For more information about Cloud Vision, read the Google Cloud Vision API
Documentation at https://cloud.google.com/vision/docs.

Creating Images

The Cloud Vision API supports a variety of image file formats, including JPEG,
PNG8, PNG24, Animated GIF (first frame only), and RAW. See
https://cloud.google.com/vision/docs/image-best-practices#image_types for the
complete list of formats. Be aware that Cloud Vision sets upper limits on file
size as well as on the total combined size of all images in a request. Reducing
your file size can significantly improve throughput; however, be careful not to
reduce image quality in the process. See
https://cloud.google.com/vision/docs/image-best-practices#image_sizing for
current file size limits.

Use NewImageFromReader or NewImageFromGCS to create images for the Cloud Vision
service. Creating an Image instance does not perform an API request.

Annotating Images

Client.Annotate is the most general method in the package. It can run multiple
detections on multiple images in a single request.

Other methods on Client run a single detection on a single image. For instance,
Client.DetectFaces will run face detection on the provided Image.

This package is experimental and subject to API changes.
*/
package vision // import "cloud.google.com/go/vision"
