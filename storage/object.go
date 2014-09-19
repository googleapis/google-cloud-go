package storage

import (
	"io"

	raw "code.google.com/p/google-api-go-client/storage/v1beta2"
)

// contentTyper implements ContentTyper to enable an
// io.ReadCloser to specify its MIME type.
type contentTyper struct {
	io.ReadCloser
	t string
}

func (c *contentTyper) ContentType() string {
	return c.t
}

// newObjectWriter returns a new objectWriter that writes to
// the file that is specified by info.Bucket and info.Name.
// Metadata changes are also reflected on the remote object
// entity, read-only fields are ignored during the write operation.
func newObjectWriter(conn *conn, info *Object) *objectWriter {
	w := &objectWriter{
		conn: conn,
		info: info,
	}
	pr, pw := io.Pipe()
	w.rc = &contentTyper{pr, info.ContentType}
	w.pw = pw
	go func() {
		// TODO(jbd): Return the inserted/updated object entity.
		_, w.err = conn.s.Objects.Insert(
			info.Bucket, info.toRawObject()).Media(w.rc).Do()
	}()
	return w
}

// objectWriter is an io.WriteCloser that opens a connection
// to update the metadata and file contents of a GCS object.
type objectWriter struct {
	conn *conn
	info *Object

	rc  io.ReadCloser
	pw  *io.PipeWriter
	err error
}

// Write writes len(p) bytes to the object. It returns the number
// of the bytes written, or an error if there is a problem occured
// during the write. It's a blocking operation, and will not return
// until the bytes are written to the underlying socket.
func (w *objectWriter) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.pw.Write(p)
}

// Close closes the writer and cleans up other resources
// used by the writer.
func (w *objectWriter) Close() error {
	if w.err != nil {
		return w.err
	}
	w.rc.Close()
	return w.pw.Close()
}

// AccessControlRule represents an access control rule entry for
// a Google Cloud Storage object.
type AccessControlRule struct {
	// Entity identifies the entity holding the current
	// rule's permissions. It could be in the form of:
	// - "user-<userId>"
	// - "user-<email>"
	// - "group-<groupId>"
	// - "group-<email>"
	// - "domain-<domain>"
	// - "project-team-<projectId>"
	// - "allUsers"
	// - "allAuthenticatedUsers"
	Entity string `json:"entity,omitempty"`

	// Role is the the access permission for the entity. Can be READER or OWNER.
	Role string `json:"role,omitempty"`
}

// Owner represents the owner of a GCS object.
type Owner struct {
	// Entity identifies the owner, it's always in the form of "user-<userId>".
	Entity string `json:"entity,omitempty"`
}

// Object represents a Google Cloud Storage (GCS) object.
type Object struct {
	// Bucket is the name of the bucket containing this GCS object.
	Bucket string `json:"bucket,omitempty"`

	// Name is the name of the object.
	Name string `json:"name,omitempty"`

	// ContentType is the MIME type of the object's content.
	ContentType string `json:"contentType,omitempty"`

	// ACL is the access control rule list for the object.
	ACL []*AccessControlRule `json:"acl,omitempty"`

	// Owner is the owner of the object. Owner is alway the original
	// uploader of the object.
	// Read-only.
	Owner *Owner `json:"owner,omitempty"`

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

	// TODO(jbd): Add timeDelete and updated.
}

func (o *Object) toRawObject() *raw.Object {
	acl := make([]*raw.ObjectAccessControl, len(o.ACL))
	for i, rule := range o.ACL {
		acl[i] = &raw.ObjectAccessControl{
			Entity: rule.Entity,
			Role:   rule.Role,
		}
	}
	return &raw.Object{
		Bucket: o.Bucket,
		Name:   o.Name,
		Acl:    acl,
	}
}

func newObject(o *raw.Object) *Object {
	if o == nil {
		return nil
	}
	acl := make([]*AccessControlRule, len(o.Acl))
	for i, rule := range o.Acl {
		acl[i] = &AccessControlRule{
			Entity: rule.Entity,
			Role:   rule.Role,
		}
	}
	return &Object{
		Bucket:          o.Bucket,
		Name:            o.Name,
		ContentType:     o.ContentType,
		ACL:             acl,
		Owner:           &Owner{Entity: o.Owner.Entity},
		ContentEncoding: o.ContentEncoding,
		Size:            o.Size,
		MD5:             []byte(o.Md5Hash),
		CRC32C:          []byte(o.Crc32c),
		MediaLink:       o.MediaLink,
		Generation:      o.Generation,
		MetaGeneration:  o.Metageneration,
	}
}
