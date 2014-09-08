package storage

import "io"

// Object represents a Google Cloud Storage (GCS) object.
type Object struct {
	// Bucket is the name of the bucket containing this GCS object.
	Bucket string `json:"bucket"`

	// Name is the name of the object.
	Name string `json:"name"`

	// ContentType is the MIME type of the object's content.
	ContentType string `json:"contentType"`

	// Size is the length of the object's content.
	// Read-only.
	Size int64 `json:"size"`

	// ContentEncoding is the encoding of the object's content.
	// Read-only.
	ContentEncoding string `json:"contentEncoding"`

	// MD5 is the MD5 hash of the data.
	// Read-only.
	MD5 []byte `json:"md5Hash"`

	// CRC32C is the CRC32C checksum of the object's content.
	// Read-only.
	CRC32C []byte `json:"crc32c"`

	// MediaLink is an URL to the object's content.
	// Read-only.
	MediaLink string `json:"mediaLink,omitempty"`

	// Metadata represents user-provided metadata, in key/value pairs.
	// It can be nil if no metadata is provided.
	Metadata map[string]string `json:"metadata"`

	// Generation is the generation version of the object's content.
	// Read-only.
	Generation int64 `json:"generation"`

	// MetaGeneration is the version of the metadata for this
	// object at this generation. This field is used for preconditions
	// and for detecting changes in metadata. A metageneration number
	// is only meaningful in the context of a particular generation
	// of a particular object. Readonly.
	MetaGeneration int64 `json:"metageneration"`

	// TODO(jbd): Add ACL and owner.
	// TODO(jbd): Add timeDelete and updated.
}

type Bucket struct {
	// Name is the name of the bucket.
	Name string `json:"name"`
}

func NewBucket(name string) (*Bucket, error) {
	// TODO(jbd): Add connection.
	return &Bucket{Name: name}, nil
}

func (b *Bucket) NewObject(name string) *Object {
	return &Object{Bucket: b.Name, Name: name}
}

// TODO(jbd): Add storage.objects.list.
// TODO(jbd): Add storage.objects.watch.

// Stat returns the meta information of an object.
func (b *Bucket) Stat(name string) (*Object, error) {
	panic("not yet impelemented")
}

// Put inserts/updates an object with the provided meta information.
func (b *Bucket) Put(name string, obj *Object) error {
	panic("not yet impelemented")
}

// Delete deletes the specified object.
func (b *Bucket) Delete(name string) error {
	panic("not yet impelemented")
}

// Copy copies the source object to the destination with the new
// meta information properties provided.
// The destination object is insterted into the current bucket
// if the destination doesn't specify another bucket name.
func (b *Bucket) Copy(srcName string, dest *Object) error {
	panic("not yet impelemented")
}

// NewReader creates a new io.ReadCloser to read the contents
// of the object identified by name from the current bucket.
func (b *Bucket) NewReader(name string) (io.ReadCloser, error) {
	panic("not yet impelemented")
}

// NewWriter creates a new io.WriteCloser to write to the GCS object
// identified by the specified name.
// If such object doesn't exist, it creates one. If obj is not nil,
// write operation also modifies the meta information of the object.
func (b *Bucket) NewWriter(name string, obj *Object) (io.WriteCloser, error) {
	panic("not yet implemented")
}
