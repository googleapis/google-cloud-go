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
func (b *Bucket) List(query *Query) (files []*File, nextQuery *Query, err error) {
	panic("not yet implemented")
}

// Copy copies the specified file to the specified destination bucket with the
// given destination name.
func (b *Bucket) Copy(name string, destName string, destBucketName string) error {
	if destBucketName == "" {
		destBucketName = b.Name
	}
	panic("not yet implemented")
}

// Remove removes a file.
func (b *Bucket) Remove(name string) error {
	panic("not yet implemented")
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
	panic("not yet implemented")
}

// Stat stats the specified file and returns metadata about it.
func (b *Bucket) Stat(name string) (file *File, err error) {
	panic("not yet implemented")
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
