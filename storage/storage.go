package storage

import (
	"io"

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

type Bucket struct {
	Name      string
	Transport oauth2.Transport
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

func (b *Bucket) List(query *Query) (files []*File, nextQuery *Query, err error) {
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

func (b *Bucket) Read(name string) (file *File, contents io.ReadCloser, err error) {
	if file, err = b.Stat(name); err != nil {
		return
	}
	panic("not yet implemented")
}

func (b *Bucket) Stat(name string) (file *File, err error) {
	panic("not yet implemented")
}

func (b *Bucket) Write(name string, file *File, contents io.ReadCloser) error {
	panic("not yet implemented")
}
