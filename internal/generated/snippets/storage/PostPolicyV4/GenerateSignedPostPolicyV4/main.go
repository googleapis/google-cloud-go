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

// [START storage_generated_storage_GenerateSignedPostPolicyV4]

package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
)

func main() {
	pv4, err := storage.GenerateSignedPostPolicyV4("my-bucket", "my-object.txt", &storage.PostPolicyV4Options{
		GoogleAccessID: "my-access-id",
		PrivateKey:     []byte("my-private-key"),

		// The upload expires in 2hours.
		Expires: time.Now().Add(2 * time.Hour),

		Fields: &storage.PolicyV4Fields{
			StatusCodeOnSuccess:    200,
			RedirectToURLOnSuccess: "https://example.org/",
			// It MUST only be a text file.
			ContentType: "text/plain",
		},

		// The conditions that the uploaded file will be expected to conform to.
		Conditions: []storage.PostPolicyV4Condition{
			// Make the file a maximum of 10mB.
			storage.ConditionContentLengthRange(0, 10<<20),
		},
	})
	if err != nil {
		// TODO: handle error.
	}

	// Now you can upload your file using the generated post policy
	// with a plain HTTP client or even the browser.
	formBuf := new(bytes.Buffer)
	mw := multipart.NewWriter(formBuf)
	for fieldName, value := range pv4.Fields {
		if err := mw.WriteField(fieldName, value); err != nil {
			// TODO: handle error.
		}
	}
	file := bytes.NewReader(bytes.Repeat([]byte("a"), 100))

	mf, err := mw.CreateFormFile("file", "myfile.txt")
	if err != nil {
		// TODO: handle error.
	}
	if _, err := io.Copy(mf, file); err != nil {
		// TODO: handle error.
	}
	if err := mw.Close(); err != nil {
		// TODO: handle error.
	}

	// Compose the request.
	req, err := http.NewRequest("POST", pv4.URL, formBuf)
	if err != nil {
		// TODO: handle error.
	}
	// Ensure the Content-Type is derived from the multipart writer.
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		// TODO: handle error.
	}
	_ = res
}

// [END storage_generated_storage_GenerateSignedPostPolicyV4]
