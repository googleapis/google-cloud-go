package udsthin

import (
	"context"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc"
	pb "poc/gothin/udsthin/gcscorepb"
)

// Writer mirrors storage.Writer. Like the native writer it buffers up to
// ChunkSize before starting the upload so that content ending within the
// first chunk goes as a single-shot upload on the JSON path.
type Writer struct {
	storage.ObjectAttrs
	ChunkSize       int
	Append          bool
	FinalizeOnClose bool

	ctx    context.Context
	o      *ObjectHandle
	stream grpc.ClientStreamingClient[pb.WriteRequest, pb.WriteObjectResponse]
	buf    []byte
	attrs  *storage.ObjectAttrs
	err    error
}

// NewWriter returns a Writer that streams object bytes through the sidecar.
func (o *ObjectHandle) NewWriter(ctx context.Context) *Writer {
	return &Writer{ctx: ctx, o: o, ChunkSize: googleapi.DefaultUploadChunkSize}
}

func (w *Writer) open(knownSize int64) error {
	if w.stream != nil || w.err != nil {
		return w.err
	}
	s, err := w.o.b.c.stub().WriteObject(w.ctx)
	if err != nil {
		w.err = mapErr(err)
		return w.err
	}
	tr := w.o.b.c.writeTransport()
	if w.Append {
		tr = GRPC
	}
	var ks *int64
	if knownSize >= 0 {
		ks = &knownSize
	}
	hdr := &pb.WriteRequest{Msg: &pb.WriteRequest_Header{Header: &pb.WriteHeader{
		Bucket:            w.o.b.name,
		Object:            w.o.object,
		ContentType:       w.ContentType,
		IfGenerationMatch: w.o.ifGenMatch(),
		KnownSize:         ks,
		ChunkSize:         uint32(w.ChunkSize),
		Transport:         tr,
	}}}
	if err := s.Send(hdr); err != nil {
		w.err = mapErr(err)
		return w.err
	}
	w.stream = s
	return nil
}

func (w *Writer) send(p []byte) error {
	// Send in <= 4 MiB proto messages matching tonic max_frame_size (4 MiB).
	const msg = 4 << 20
	for len(p) > 0 {
		k := min(len(p), msg)
		if err := w.stream.Send(&pb.WriteRequest{Msg: &pb.WriteRequest_Data{Data: p[:k]}}); err != nil {
			_, rerr := w.stream.CloseAndRecv()
			if rerr == nil {
				rerr = err
			}
			w.err = mapErr(rerr)
			return w.err
		}
		p = p[k:]
	}
	return nil
}

// Write appends bytes to the upload stream.
func (w *Writer) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.stream == nil {
		if w.ChunkSize > 0 && len(w.buf)+len(p) < w.ChunkSize {
			w.buf = append(w.buf, p...)
			return len(p), nil
		}
		if err := w.open(-1); err != nil {
			return 0, err
		}
		if err := w.send(w.buf); err != nil {
			return 0, err
		}
		w.buf = nil
	}
	if err := w.send(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close flushes any buffered bytes, commits the object, and populates Attrs().
func (w *Writer) Close() error {
	if w.err != nil {
		return w.err
	}
	if w.stream == nil {
		if err := w.open(int64(len(w.buf))); err != nil {
			return err
		}
		if err := w.send(w.buf); err != nil {
			return err
		}
		w.buf = nil
	}
	resp, err := w.stream.CloseAndRecv()
	if err != nil {
		w.err = mapErr(err)
		return w.err
	}
	w.attrs = &storage.ObjectAttrs{
		Bucket:      w.o.b.name,
		Name:        w.o.object,
		Size:        resp.Size,
		Generation:  resp.Generation,
		CRC32C:      resp.Crc32C,
		ContentType: w.ContentType,
	}
	return nil
}

// Attrs returns the finalized object attributes after Close succeeds.
func (w *Writer) Attrs() *storage.ObjectAttrs { return w.attrs }
