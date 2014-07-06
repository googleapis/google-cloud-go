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
	"time"
)

type Owner struct {
	entity   string `json:"entity"`
	entityID string `json:"entityId"`
}

type ACL struct {
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	Email      string `json:"email"`
	Entity     string `json:"entity"`
	EntityID   string `json:"entityId"`
	Generation int64  `json:"generation"`
	Role       string `json:"role"`
}

// TODO: File should implement os.FileInfo

type File struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BucketName string `json:"bucket"`

	ACL   []*ACL `json:"acl"`
	Owner *Owner `json:"owner"`

	CacheControl    string `json:"cacheControl"`
	ComponentCount  int64  `json:"componentCount"`
	ContentEncoding string `json:"contentEncoding"`
	ContentType     string `json:"contentType"`
	ContentLanuage  string `json:"contentLanguage"`
	CRC32c          string `json:"crc32c"`
	MD5Hash         string `json:"md5hash"`
	Size            uint64 `json:"size,string"`
	Etag            string `json:"etag"`

	Metadata  map[string]string `json:"metadata"`
	MediaLink string            `json:"mediaLink"`

	DeleteTime time.Time `json:"timeDeleted"`
	UpdateTime time.Time `json:"updated"`
}

type Query struct {
	Delimeter  string `json:"delimeter"`
	MaxResults uint64 `json:"maxResults"`
	Prefix     string `json:"prefix"`
	PageToken  string `json:"pageToken"`
	Versions   bool   `json:"versions"`
}

type listResponse struct {
	Items         []*File `json:"items"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
}
