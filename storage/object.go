package storage

import (
	"io"

	raw "code.google.com/p/google-api-go-client/storage/v1beta2"
)

func newObjectWriter(conn *conn, info *ObjectInfo) *objectWriter {
	w := &objectWriter{
		conn: conn,
		info: info,
	}
	pr, pw := io.Pipe()
	w.pr = pr
	w.pw = pw
	return w
}

type objectWriter struct {
	conn *conn
	info *ObjectInfo

	pr *io.PipeReader
	pw *io.PipeWriter
}

func (w *objectWriter) Write(p []byte) (n int, err error) {
	panic("not yet implemented")
}

func (w *objectWriter) Close() error {
	panic("not yet implemented")
}

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
	// of a particular object.
	// Read-only.
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
