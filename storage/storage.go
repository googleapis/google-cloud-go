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
	"os"

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
	Name      string
	Transport oauth2.Transport
}

// NewBucket returns a new bucket whose calls will be authorized
// with the provided email and private key.
func NewBucket(bucketName, email, pemFilename string) (bucket *Bucket, err error) {
	conf, err := google.NewServiceAccountConfig(&oauth2.JWTOptions{
		Email:       email,
		PemFilename: pemFilename,
		Scopes:      requiredScopes,
	})
	if err != nil {
		return
	}
	bucket = &Bucket{
		Name:      bucketName,
		Transport: conf.NewTransport(),
	}
	return
}

// List lists files from the bucket. It returns a non-nil nextQuery
// if more pages are available.
func (b *Bucket) List(query *Query) (files []*File, next *Query, err error) {
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
	u, err := url.Parse(storageBaseURL + "/b/" + b.Name + "/o" + "?" + params.Encode())
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

// Copy copies the specified file with the specified file info.
func (b *Bucket) Copy(name string, destFile *File) error {
	if destFile.Name == "" {
		return errors.New("storage: destination file should have a name")
	}
	if destFile.BucketName == "" {
		destFile.BucketName = b.Name
	}
	client := &client{transport: b.Transport}
	u, err := url.Parse(storageBaseURL + "/b/" + b.Name + "/o/" + name + "/copyTo/b/" + destFile.BucketName + "/o/" + destFile.Name)
	if err != nil {
		return err
	}
	return client.Do("POST", u, destFile, nil)
}

// Remove removes a file.
func (b *Bucket) Remove(name string) error {
	client := &client{transport: b.Transport}
	u, err := url.Parse(storageBaseURL + "/b/" + b.Name + "/o/" + name)
	if err != nil {
		return err
	}
	return client.Do("DELETE", u, nil, nil)
}

// Read reads the specified file.
func (b *Bucket) Read(name string) (file *File, contents io.ReadCloser, err error) {
	if file, err = b.Stat(name); err != nil {
		return
	}
	if file.MediaLink == "" {
		err = errors.New("storage: file doesn't contain blob contents")
		return
	}
	// make a request to read file blob
	u, err := url.Parse(file.MediaLink)
	if err != nil {
		return nil, nil, err
	}
	client := &client{transport: b.Transport}
	contents, err = client.RespBody("GET", u)
	return
}

// Stat stats the specified file and returns metadata about it.
func (b *Bucket) Stat(name string) (file *File, err error) {
	client := &client{transport: b.Transport}
	u, err := url.Parse(storageBaseURL + "/b/" + b.Name + "/o/" + name)
	if err != nil {
		return nil, err
	}
	file = &File{}
	if err = client.Do("GET", u, nil, file); err != nil {
		return nil, err
	}
	return file, nil
}

// TODO: Support resumable uploads. DefaultWriter, ResumableWriter, etc.

// Write writes the provided contents to the remote destination
// identified with name. You can provide additional metadata
// information if you would like to make metadata changes.
// Write inserts the file if there is no file identified with name,
// updates the existing if there exists a file.
func (b *Bucket) Write(name string, file *File, contents io.Reader) error {
	panic("not yet implemented")
}

// WriteFile writes the contents of the provided file to the remote
// destination identified with name.
func (b *Bucket) WriteFile(name, file *File, srcFile *os.File) error {
	panic("not yet implemented")
}
