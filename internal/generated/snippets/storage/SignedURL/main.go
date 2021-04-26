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

// [START storage_generated_storage_SignedURL]

package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"cloud.google.com/go/storage"
)

func main() {
	pkey, err := ioutil.ReadFile("my-private-key.pem")
	if err != nil {
		// TODO: handle error.
	}
	url, err := storage.SignedURL("my-bucket", "my-object", &storage.SignedURLOptions{
		GoogleAccessID: "xxx@developer.gserviceaccount.com",
		PrivateKey:     pkey,
		Method:         "GET",
		Expires:        time.Now().Add(48 * time.Hour),
	})
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(url)
}

// [END storage_generated_storage_SignedURL]
