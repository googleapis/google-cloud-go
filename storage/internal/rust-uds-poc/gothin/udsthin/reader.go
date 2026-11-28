package udsthin

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/grpc"
	pb "poc/gothin/udsthin/gcscorepb"
)

// Reader mirrors storage.Reader (io.ReadCloser + Attrs/Remain).
//
// Copies on the Go side per byte: kernel -> gRPC receive buffer (1), then with
// ZeroCopyDecode the chunk pieces alias that buffer and Read copies once more
// into p (2). Without it, proto.Unmarshal adds a third copy.
type Reader struct {
	Attrs  storage.ReaderObjectAttrs
	stream grpc.ClientStream
	cancel context.CancelFunc
	zc     bool
	cur    *rawChunk
	pbcur  *pb.ReadChunk
	parts  [][]byte
	remain int64
	eof    bool
}

var (
	readObjectDesc = &grpc.StreamDesc{StreamName: "ReadObject", ServerStreams: true}
	readRangeDesc  = &grpc.StreamDesc{StreamName: "ReadRange", ServerStreams: true}
)

// NewReader opens a full-object reader.
func (o *ObjectHandle) NewReader(ctx context.Context, opts ...storage.ReaderOption) (*Reader, error) {
	return o.NewRangeReader(ctx, 0, -1, opts...)
}

// NewRangeReader opens a range reader over [offset, offset+length).
func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64, _ ...storage.ReaderOption) (*Reader, error) {
	if offset < 0 && length >= 0 {
		return nil, fmt.Errorf("storage: invalid offset %d < 0 requires negative length", offset)
	}
	cctx, cancel := context.WithCancel(ctx)
	stream, err := o.b.c.conn().NewStream(cctx, readObjectDesc, "/gcscore.v1.GcsCore/ReadObject")
	if err != nil {
		cancel()
		return nil, mapErr(err)
	}
	req := &pb.ReadObjectRequest{
		Bucket:            o.b.name,
		Object:            o.object,
		Offset:            offset,
		Length:            length,
		Generation:        o.gen,
		IfGenerationMatch: o.ifGenMatch(),
		Transport:         o.b.c.opts.Transport,
		ChunkSize:         o.b.c.opts.ChunkSize,
	}
	if err := stream.SendMsg(req); err != nil {
		cancel()
		return nil, mapErr(err)
	}
	_ = stream.CloseSend()
	r := &Reader{stream: stream, cancel: cancel, zc: o.b.c.opts.ZeroCopyDecode}
	meta, err := r.recv()
	if err != nil && err != io.EOF {
		cancel()
		return nil, mapErr(err)
	}
	if m := meta; m != nil {
		r.Attrs = storage.ReaderObjectAttrs{
			Size:            m.Size,
			StartOffset:     max(offset, 0),
			ContentType:     m.ContentType,
			ContentEncoding: m.ContentEncoding,
			Generation:      m.Generation,
			Metageneration:  m.Metageneration,
			CRC32C:          m.Crc32C,
		}
		if length >= 0 {
			r.remain = min(length, m.Size-max(offset, 0))
		} else {
			r.remain = m.Size - max(offset, 0)
		}
	}
	return r, nil
}

func (r *Reader) recv() (*pb.ObjectMeta, error) {
	if r.zc {
		if r.cur != nil && r.cur.release != nil {
			r.cur.release()
			r.cur = nil
		}
		rc := &rawChunk{}
		if err := r.stream.RecvMsg(rc); err != nil {
			if err == io.EOF {
				r.eof = true
			}
			return nil, err
		}
		r.cur = rc
		r.parts = rc.parts
		return rc.meta, nil
	}
	c := &pb.ReadChunk{}
	if err := r.stream.RecvMsg(c); err != nil {
		if err == io.EOF {
			r.eof = true
		}
		return nil, err
	}
	r.pbcur = c
	r.parts = c.Data
	return c.Meta, nil
}

// Read copies the next bytes from the stream into p.
func (r *Reader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if len(r.parts) == 0 {
			if r.eof {
				break
			}
			if _, err := r.recv(); err != nil {
				if err == io.EOF {
					break
				}
				return n, mapErr(err)
			}
			continue
		}
		k := copy(p[n:], r.parts[0])
		if k == len(r.parts[0]) {
			r.parts = r.parts[1:]
		} else {
			r.parts[0] = r.parts[0][k:]
		}
		n += k
	}
	r.remain -= int64(n)
	if n == 0 && r.eof {
		return 0, io.EOF
	}
	return n, nil
}

// Remain returns the number of unread bytes remaining in the range.
func (r *Reader) Remain() int64 { return r.remain }

// Close cancels the underlying stream and releases any pinned receive buffer.
func (r *Reader) Close() error {
	r.cancel()
	if r.cur != nil && r.cur.release != nil {
		r.cur.release()
		r.cur = nil
	}
	return nil
}

// Opened represents an open BidiReadObject handle held in the Rust sidecar for
// low-latency random range reads (Rapid/MRD workloads).
type Opened struct {
	c      *Client
	handle uint64
	Attrs  storage.ReaderObjectAttrs
}

// Open opens a persistent BidiReadObject descriptor on the Rust sidecar.
func (o *ObjectHandle) Open(ctx context.Context) (*Opened, error) {
	resp, err := o.b.c.stub().OpenObject(ctx, &pb.OpenObjectRequest{
		Bucket:     o.b.name,
		Object:     o.object,
		Generation: o.gen,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	m := resp.Meta
	return &Opened{
		c:      o.b.c,
		handle: resp.Handle,
		Attrs: storage.ReaderObjectAttrs{
			Size:           m.Size,
			Generation:     m.Generation,
			Metageneration: m.Metageneration,
			ContentType:    m.ContentType,
		},
	}, nil
}

// ReadAt reads [off, off+len(p)) from the open descriptor into p.
func (op *Opened) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := op.c.conn().NewStream(cctx, readRangeDesc, "/gcscore.v1.GcsCore/ReadRange")
	if err != nil {
		return 0, mapErr(err)
	}
	req := &pb.ReadRangeRequest{
		Handle:    op.handle,
		Offset:    off,
		Length:    int64(len(p)),
		ChunkSize: op.c.opts.ChunkSize,
	}
	if err := stream.SendMsg(req); err != nil {
		return 0, mapErr(err)
	}
	_ = stream.CloseSend()
	n := 0
	for n < len(p) {
		var parts [][]byte
		var release func()
		if op.c.opts.ZeroCopyDecode {
			rc := &rawChunk{}
			if err := stream.RecvMsg(rc); err != nil {
				if err == io.EOF {
					break
				}
				return n, mapErr(err)
			}
			parts, release = rc.parts, rc.release
		} else {
			c := &pb.ReadChunk{}
			if err := stream.RecvMsg(c); err != nil {
				if err == io.EOF {
					break
				}
				return n, mapErr(err)
			}
			parts = c.Data
		}
		for _, part := range parts {
			n += copy(p[n:], part)
		}
		if release != nil {
			release()
		}
	}
	if n < len(p) {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

// Close releases the open object descriptor on the sidecar.
func (op *Opened) Close(ctx context.Context) error {
	_, err := op.c.stub().CloseObject(ctx, &pb.CloseObjectRequest{Handle: op.handle})
	return mapErr(err)
}

// MultiRangeDownloader mirrors storage.MultiRangeDownloader so callers written
// against the Go veneer API can use BidiReadObject over UDS without code changes.
type MultiRangeDownloader struct {
	Attrs storage.ReaderObjectAttrs
	ctx   context.Context
	op    *Opened
}

// NewMultiRangeDownloader opens a BidiReadObject session on the Rust sidecar
// and returns a MultiRangeDownloader with the same signature as cloud.google.com/go/storage.
func (o *ObjectHandle) NewMultiRangeDownloader(ctx context.Context, _ ...storage.ReaderOption) (*MultiRangeDownloader, error) {
	op, err := o.Open(ctx)
	if err != nil {
		return nil, err
	}
	return &MultiRangeDownloader{Attrs: op.Attrs, ctx: ctx, op: op}, nil
}

// Add reads [offset, offset+length) into output and invokes callback upon completion.
func (m *MultiRangeDownloader) Add(output io.Writer, offset, length int64, callback func(int64, int64, error)) {
	buf := make([]byte, length)
	n, err := m.op.ReadAt(m.ctx, buf, offset)
	if n > 0 {
		if _, werr := output.Write(buf[:n]); werr != nil && err == nil {
			err = werr
		}
	}
	if callback != nil {
		callback(offset, int64(n), err)
	}
}

// Wait blocks until all queued downloads finish.
func (m *MultiRangeDownloader) Wait() error { return nil }

// Close closes the underlying BidiReadObject descriptor on the sidecar.
func (m *MultiRangeDownloader) Close() error { return m.op.Close(m.ctx) }

// Error returns any session-level error.
func (m *MultiRangeDownloader) Error() error { return nil }
