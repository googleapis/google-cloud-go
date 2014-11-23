// Copyright 2014 Google Inc. All Rights Reserved.
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

package storage

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"testing"
	"time"
)

func TestSignedURL(t *testing.T) {
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	url, err := SignedURL("bucket-name", "object-name", &SignedURLOptions{
		ClientID:    "xxx@clientid",
		PrivateKey:  dummyKey("rsa"),
		Method:      "GET",
		MD5:         []byte("202cb962ac59075b964b07152d234b70"),
		Expires:     expires,
		ContentType: "application/json",
		Headers:     []string{"x-header1", "x-header2"},
	})
	if err != nil {
		t.Error(err)
	}
	want := "http://storage.googleapis.com/bucket-name/object-name?" +
		"GoogleAccessId=xxx@clientid&Expires=1033570800&Signature=" +
		"ITqNWQHr7ayIj+0Ds5/zUT2cWMQQouuFmu6L11Zd3kfNKvm3sjyGIzOgZ" +
		"sSUoter1SxP7BcrCzgqIZ9fQmgQnuIpqqLL4kcGmTbKsQS6hTknpJM/2l" +
		"S4NY6UH1VXBgm2Tce28kz8rnmqG6svcGvtWuOgJsETeSIl1R9nAEIDCEq" +
		"ZJzoOiru+ODkHHkpoFjHWAwHugFHX+9EX4SxaytiN3oEy48HpYGWV0Ih8" +
		"NvU1hmeWzcLr41GnTADeCn7Eg/b5H2GCNO70Cz+w2fn+ofLCUeRYQd/hE" +
		"S8oocv5kpHZkstc8s8uz3aKMsMauzZ9MOmGy/6VULBgIVvi6aAwEBIYOw=="
	if url != want {
		t.Fatalf("Unexpected signed URL; found %v", url)
	}
}

func TestSignedURL_PEMPrivateKey(t *testing.T) {
	expires, _ := time.Parse(time.RFC3339, "2002-10-02T10:00:00-05:00")
	url, err := SignedURL("bucket-name", "object-name", &SignedURLOptions{
		ClientID:    "xxx@clientid",
		PrivateKey:  dummyKey("pem"),
		Method:      "GET",
		MD5:         []byte("202cb962ac59075b964b07152d234b70"),
		Expires:     expires,
		ContentType: "application/json",
		Headers:     []string{"x-header1", "x-header2"},
	})
	if err != nil {
		t.Error(err)
	}
	want := "http://storage.googleapis.com/bucket-name/object-name?" +
		"GoogleAccessId=xxx@clientid&Expires=1033570800&Signature=" +
		"B7XkS4dfmVDoe/oDeXZkWlYmg8u2kI0SizTrzL5+9RmKnb5j7Kf34DZJL" +
		"8Hcjr1MdPFLNg2QV4lEH86Gqgqt/v3jFOTRl4wlzcRU/vV5c5HU8MqW0F" +
		"Z0IDbqod2RdsMONLEO6yQWV2HWFrMLKl2yMFlWCJ47et+FaHe6v4ZEBc0="
	if url != want {
		t.Fatalf("Unexpected signed URL; found %v", url)
	}
}

func TestSignedURL_MissingOptions(t *testing.T) {
	pk := dummyKey("rsa")
	var tests = []struct {
		opts   *SignedURLOptions
		errMsg string
	}{
		{
			&SignedURLOptions{},
			"missing required credentials",
		},
		{
			&SignedURLOptions{ClientID: "client_id"},
			"missing required credentials",
		},
		{
			&SignedURLOptions{
				ClientID:   "client_id",
				PrivateKey: pk,
			},
			"missing required method",
		},
		{
			&SignedURLOptions{
				ClientID:   "client_id",
				PrivateKey: pk,
				Method:     "PUT",
			},
			"missing required expires",
		},
	}
	for _, test := range tests {
		_, err := SignedURL("bucket", "name", test.opts)
		if !strings.Contains(err.Error(), test.errMsg) {
			t.Errorf("expected err: %v, found: %v", test.errMsg, err)
		}
	}
}

func dummyKey(kind string) []byte {
	slurp, err := ioutil.ReadFile(fmt.Sprintf("./testdata/dummy_%s", kind))
	if err != nil {
		log.Fatal(err)
	}
	return slurp
}
