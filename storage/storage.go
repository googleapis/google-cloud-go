package storage

import "io"

// Object represents a Google Cloud Storage (GCS) object.
type Object struct {
	// Bucket is the name of the bucket containing this GCS object.
	Bucket string `json:"bucket,omitempty"`

	// Name is the name of the object.
	Name string `json:"name,omitempty"`

	// ContentType is the MIME type of the object's content.
	ContentType string `json:"contentType,omitempty"`

	// Size is the length of the object's content.
	// Read-only.
	Size int64 `json:"size,omitempty"`

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

type Bucket struct {
	// Name is the name of the bucket.
	Name string `json:"name,omitempty"`
}

type Storage struct {
	// TODO(jbd): Add connection
}

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.insert.
// TODO(jbd): Add storage.buckets.update.
// TODO(jbd): Add storage.buckets.delete.
// TODO(jbd): Add storage.objects.list.
// TODO(jbd): Add storage.objects.watch.

// GetBucket returns the specified bucket.
func (s *Storage) GetBucket(name string) (*Bucket, error) {
	panic("not yet implemented")
}

// GetObject returns the meta information of an object.
func (s *Storage) GetObject(bucket, name string) (*Object, error) {
	panic("not yet impelemented")
}

// PutObject inserts/updates an object with the provided meta information.
func (s *Storage) PutObject(bucket, name string, obj *Object) error {
	panic("not yet impelemented")
}

// DeleteObject deletes the specified object.
func (s *Storage) DeleteObject(bucket, name string) error {
	panic("not yet impelemented")
}

// CopyObject copies the source object to the destination with the new
// meta information properties provided.
// The destination object is insterted into the source bucket
// if the destination doesn't specify another bucket name.
func (s *Storage) CopyObject(bucket, name string, dest *Object) error {
	panic("not yet impelemented")
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object.
func (s *Storage) NewReader(bucket, name string) (io.ReadCloser, error) {
	panic("not yet impelemented")
}

// NewWriter creates a new io.WriteCloser to write to the GCS object
// identified by the specified bucket and name.
// If such object doesn't exist, it creates one. If obj is not nil,
// write operation also modifies the meta information of the object.
func (s *Storage) NewWriter(bucket, name string, obj *Object) (io.WriteCloser, error) {
	panic("not yet implemented")
}
