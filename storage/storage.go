package storage

import (
	"io"
	"time"

	"github.com/golang/oauth2"
	"github.com/golang/oauth2/google"
)

var requiredScopes = []string{}

type Bucket struct {
	Name      string
	Transport oauth2.Transport
}

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

type Metadata struct {
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

	ID         string `json:"id"`
	Name       string `json:"name"`
	BucketName string `json:"bucket"`

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

func (b *Bucket) List(query *Query) (objects []*Metadata, nextQuery *Query, err error) {
	panic("not yet implemented")
}

func (b *Bucket) Copy(name string, destName string, destBucketName string) error {
	if destBucketName == "" {
		destBucketName = b.Name
	}
	panic("not yet implemented")
}

func (b *Bucket) Delete(name string) error {
	panic("not yet implemented")
}

func (b *Bucket) Read(name string) (metadata *Metadata, contents io.ReadCloser, err error) {
	panic("not yet implemented")
}

func (b *Bucket) ReadMetadata(name string) (metadata *Metadata, err error) {
	panic("not yet implemented")
}

func (b *Bucket) Write(name string, metadata *Metadata, contents io.ReadCloser) error {
	panic("not yet implemented")
}
