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
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
)

const (
	storageBaseURL       = "https://www.googleapis.com/storage/v1"
	storageBaseUploadURL = "https://www.googleapis.com/upload/storage/v1"
)

var requiredScopes = []string{
	"https://www.googleapis.com/auth/devstorage.full_control",
}

// Bucket represents a Google Cloud Storage bucket.
// See the guide on https://developers.google.com/storage to
// create a bucket.
type Bucket struct {
	name      string
	Transport *oauth2.Transport
}

// NewBucket returns a new bucket whose calls will be authorized
// with the provided email and private key.
func NewBucket(bucketName, email string, privateKey []byte) (*Bucket, error) {
	conf, err := google.NewServiceAccountConfig(&oauth2.JWTOptions{
		Email:      email,
		PrivateKey: privateKey,
		Scopes:     requiredScopes,
	})
	if err != nil {
		return nil, err
	}
	return &Bucket{
		name:      bucketName,
		Transport: conf.NewTransport(),
	}, nil
}

// List lists files from the bucket. It returns a non-nil nextQuery
// if more pages are available.
func (b *Bucket) List(query *Query) (objects []*Object, next *Query, err error) {
	client := &client{transport: b.Transport}
	params := url.Values{}
	if query != nil {
		params.Add("delimeter", query.Delimeter)
		if query.MaxResults > 0 {
			params.Add("maxResults", fmt.Sprintf("%d", query.MaxResults))
		}
		params.Add("pageToken", query.PageToken)
		params.Add("prefix", query.Prefix)
		params.Add("versions", fmt.Sprintf("%t", query.Versions))
	}
	u, err := url.Parse(storageBaseURL + "/b/" + b.name + "/o" + "?" + params.Encode())
	if err != nil {
		return
	}
	resp := &listResponse{}
	if err = client.Do("GET", u, nil, resp); err != nil {
		return
	}
	if resp.NextPageToken != "" {
		next = &Query{
			Delimeter:  query.Delimeter,
			MaxResults: query.MaxResults,
			PageToken:  resp.NextPageToken,
			Prefix:     query.Prefix,
			Versions:   query.Versions,
		}
	}
	return resp.Items, next, nil
}

func (b *Bucket) Copy(objectName string, destObject *Object) error {
	if destObject.Name == "" {
		return errors.New("storage: destination file should have a name")
	}
	if destObject.BucketName == "" {
		destObject.BucketName = b.name
	}
	client := &client{transport: b.Transport}
	u, err := url.Parse(storageBaseURL + "/b/" + b.name + "/o/" + objectName + "/copyTo/b/" + destObject.BucketName + "/o/" + destObject.Name)
	if err != nil {
		return err
	}
	return client.Do("POST", u, destObject, nil)
}

func (b *Bucket) Delete(objectName string) error {
	client := &client{transport: b.Transport}
	u, err := url.Parse(storageBaseURL + "/b/" + b.name + "/o/" + objectName)
	if err != nil {
		return err
	}
	return client.Do("DELETE", u, nil, nil)
}

func (b *Bucket) Read(objectName string) (object *Object, contents io.ReadCloser, err error) {
	if object, err = b.Stat(objectName); err != nil {
		return
	}
	if object.MediaLink == "" {
		err = errors.New("storage: file doesn't contain blob contents")
		return
	}
	u, err := url.Parse(object.MediaLink)
	if err != nil {
		return nil, nil, err
	}
	client := &client{transport: b.Transport}
	contents, err = client.RespBody("GET", u)
	return
}

// Stat stats the specified file and returns metadata about it.
func (b *Bucket) Stat(objectName string) (*Object, error) {
	u, err := url.Parse(storageBaseURL + "/b/" + b.name + "/o/" + objectName)
	if err != nil {
		return nil, err
	}
	o := &Object{}
	client := &client{transport: b.Transport}
	if err = client.Do("GET", u, nil, o); err != nil {
		return nil, err
	}
	return o, nil
}
