// Package storage is a Google Cloud Storage client.
package storage

import (
	"errors"
	"io"
	"net/http"

	raw "code.google.com/p/google-api-go-client/storage/v1beta2"
)

// OAuth 2.0 scopes used by this API.
const (
	// Manage your data and permissions in Google Cloud Storage
	ScopeFullControl = raw.DevstorageFull_controlScope

	// View your data in Google Cloud Storage
	ScopeReadOnly = raw.DevstorageRead_onlyScope

	// Manage your data in Google Cloud Storage
	ScopeReadWrite = raw.DevstorageRead_writeScope
)

// ObjectInfo represents a Google Cloud Storage (GCS) object.
type ObjectInfo struct {
	// Bucket is the name of the bucket containing this GCS object.
	Bucket string `json:"bucket,omitempty"`

	// Name is the name of the object.
	Name string `json:"name,omitempty"`

	// ContentType is the MIME type of the object's content.
	ContentType string `json:"contentType,omitempty"`

	// Size is the length of the object's content.
	// Read-only.
	Size uint64 `json:"size,omitempty"`

	// ContentEncoding is the encoding of the object's content.
	// Read-only.
	ContentEncoding string `json:"contentEncoding,omitempty"`

	// MD5 is the MD5 hash of the data.
	// Read-only.
	MD5 []byte `json:"md5Hash,omitempty"`

	// CRC32C is the CRC32C checksum of the object's content.
	// Read-only.
	CRC32C []byte `json:"crc32c,omitempty"`

	// MediaLink is an URL to the object's content.
	// Read-only.
	MediaLink string `json:"mediaLink,omitempty"`

	// Metadata represents user-provided metadata, in key/value pairs.
	// It can be nil if no metadata is provided.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Generation is the generation version of the object's content.
	// Read-only.
	Generation int64 `json:"generation,omitempty"`

	// MetaGeneration is the version of the metadata for this
	// object at this generation. This field is used for preconditions
	// and for detecting changes in metadata. A metageneration number
	// is only meaningful in the context of a particular generation
	// of a particular object. Readonly.
	MetaGeneration int64 `json:"metageneration,omitempty"`

	// TODO(jbd): Add ACL and owner.
	// TODO(jbd): Add timeDelete and updated.
}

func (o *ObjectInfo) toRawObject() *raw.Object {
	// TODO(jbd): add ACL and owner
	return &raw.Object{
		Bucket:      o.Bucket,
		Name:        o.Name,
		ContentType: o.ContentType,
	}
}

func newObjectInfo(o *raw.Object) *ObjectInfo {
	if o == nil {
		return nil
	}
	return &ObjectInfo{
		Bucket:          o.Bucket,
		Name:            o.Name,
		ContentType:     o.ContentType,
		ContentEncoding: o.ContentEncoding,
		Size:            o.Size,
		MD5:             []byte(o.Md5Hash),
		CRC32C:          []byte(o.Crc32c),
		MediaLink:       o.MediaLink,
		Generation:      o.Generation,
		MetaGeneration:  o.Metageneration,
	}
}

type BucketInfo struct {
	// Name is the name of the bucket.
	Name string `json:"name,omitempty"`
}

type Bucket struct {
	name string
	s    *raw.Service
}

type Client struct {
	s *raw.Service
}

func New(tr http.RoundTripper) (*Client, error) {
	return NewWithClient(&http.Client{Transport: tr})
}

func NewWithClient(c *http.Client) (*Client, error) {
	s, err := raw.New(c)
	if err != nil {
		return nil, err
	}
	return &Client{s: s}, nil
}

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.insert.
// TODO(jbd): Add storage.buckets.update.
// TODO(jbd): Add storage.buckets.delete.

// TODO(jbd): Add storage.objects.list.

// GetBucketInfo returns the specified bucket.
func (c *Client) GetBucketInfo(name string) (*BucketInfo, error) {
	panic("not yet implemented")
}

func (c *Client) NewBucket(name string) *Bucket {
	return &Bucket{name: name, s: c.s}
}

// Stat returns the meta information of an object.
func (b *Bucket) Stat(name string) (*ObjectInfo, error) {
	o, err := b.s.Objects.Get(b.name, name).Do()
	if err != nil {
		// TODO(jbd): If 404, return ErrNotExists
		return nil, err
	}
	return newObjectInfo(o), nil
}

// Put inserts/updates an object with the provided meta information.
func (b *Bucket) Put(name string, info *ObjectInfo) (*ObjectInfo, error) {
	o, err := b.s.Objects.Insert(b.name, info.toRawObject()).Do()
	if err != nil {
		return nil, err
	}
	return newObjectInfo(o), nil
}

// Delete deletes the specified object.
func (b *Bucket) Delete(name string) error {
	return b.s.Objects.Delete(b.name, name).Do()
}

// Copy copies the source object to the destination with the new
// meta information properties provided.
// The destination object is inserted into the source bucket
// if the destination object doesn't specify another bucket name.
func (b *Bucket) Copy(name string, dest *ObjectInfo) (*ObjectInfo, error) {
	if dest.Name == "" {
		return nil, errors.New("storage: missing dest name")
	}
	destBucket := dest.Bucket
	if destBucket == "" {
		destBucket = b.name
	}
	o, err := b.s.Objects.Copy(
		b.name, name, destBucket, dest.Name, dest.toRawObject()).Do()
	if err != nil {
		// TODO(jbd): Return ErrNotExists if 404.
		return nil, err
	}
	return newObjectInfo(o), nil
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object.
func (b *Bucket) NewReader(name string) (io.ReadCloser, error) {
	panic("not yet impelemented")
}

// NewWriter creates a new io.WriteCloser to write to the GCS object
// identified by the specified bucket and name.
// If such object doesn't exist, it creates one. If info is not nil,
// write operation also modifies the meta information of the object.
func (b *Bucket) NewWriter(name string, info *ObjectInfo) (io.WriteCloser, error) {
	panic("not yet implemented")
}
