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
	Size            int64  `json:"size"`
	Etag            string `json:"etag"`
	Generation      int64  `json:"generation"`

	Metadata       map[string]string `json:"metadata"`
	MetaGeneration int64             `json:"metageneration"`
	MediaLink      string            `json:"mediaLink"`

	DeleteTime time.Time `json:"timeDeleted"`
	UpdateTime time.Time `json:"updated"`
}

type Query struct {
	Delimeter  string `json:"delimeter"`
	MaxResults uint   `json:"maxResults"`
	Prefix     string `json:"prefix"`
	PageToken  string `json:"pageToken"`
	Versions   bool   `json:"versions"`
}

type listResponse struct {
	items []*File `json:"items"`
}
