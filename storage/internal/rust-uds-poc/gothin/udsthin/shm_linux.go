//go:build linux

package udsthin

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"

	"cloud.google.com/go/storage"
	pb "poc/gothin/udsthin/gcscorepb"
)

// ShmRegion is a memfd created by the sidecar, received over <socket>.shm
// with SCM_RIGHTS and mapped read-only here. Both processes see the same
// physical pages: the sidecar's write into the region IS the delivery.
type ShmRegion struct {
	ID   uint64
	Mem  []byte // mmap'd, PROT_READ, MAP_SHARED
	conn *net.UnixConn
	fd   int
}

// OpenShm connects to the side channel and maps the region the server hands out.
func (c *Client) OpenShm() (*ShmRegion, error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: c.side.Socket + ".shm", Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("shm side channel: %w", err)
	}
	buf := make([]byte, 16)
	oob := make([]byte, syscall.CmsgSpace(4))
	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if n != 16 {
		conn.Close()
		return nil, errors.New("short shm handshake")
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) == 0 {
		conn.Close()
		return nil, fmt.Errorf("no SCM_RIGHTS: %v", err)
	}
	fds, err := syscall.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) == 0 {
		conn.Close()
		return nil, fmt.Errorf("parse rights: %v", err)
	}
	id := binary.LittleEndian.Uint64(buf[:8])
	size := binary.LittleEndian.Uint64(buf[8:])
	mem, err := syscall.Mmap(fds[0], 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("mmap memfd: %w", err)
	}
	return &ShmRegion{ID: id, Mem: mem, conn: conn, fd: fds[0]}, nil
}

func (r *ShmRegion) Close() {
	syscall.Munmap(r.Mem)
	syscall.Close(r.fd)
	r.conn.Close() // server drops the region when it sees EOF
}

// ShmReader reads an object whose bytes the sidecar places directly in the
// shared region. Read(p) performs exactly one copy (shared page -> p), the
// same count as the native SDK's TLS-record -> p copy. Next() lends the
// shared slice itself: zero copies on the Go side (valid until the next call).
type ShmReader struct {
	Attrs  storage.ReaderObjectAttrs
	region *ShmRegion
	stream pb.GcsCore_ReadObjectShmClient
	cancel context.CancelFunc
	cur    []byte
	eof    bool
}

// NewShmReader reads the whole object (must fit in region[base:base+cap]).
func (o *ObjectHandle) NewShmReader(ctx context.Context, region *ShmRegion, base, capacity uint64) (*ShmReader, error) {
	cctx, cancel := context.WithCancel(ctx)
	RPCs.Add(1)
	stream, err := o.b.c.stub().ReadObjectShm(cctx, &pb.ReadObjectShmRequest{
		Read: &pb.ReadObjectRequest{Bucket: o.b.name, Object: o.object, Offset: 0, Length: -1, Generation: o.gen,
			IfGenerationMatch: o.ifGenMatch(), Transport: o.b.c.opts.Transport, ChunkSize: o.b.c.opts.ChunkSize},
		Region: region.ID, BaseOffset: base, Capacity: capacity,
	})
	if err != nil {
		cancel()
		return nil, mapErr(err)
	}
	first, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, mapErr(err)
	}
	r := &ShmReader{region: region, stream: stream, cancel: cancel}
	r.cur = region.Mem[first.Offset : first.Offset+first.Len]
	if m := first.Meta; m != nil {
		r.Attrs = storage.ReaderObjectAttrs{Size: m.Size, ContentType: m.ContentType, Generation: m.Generation, Metageneration: m.Metageneration, CRC32C: m.Crc32C}
	}
	return r, nil
}

// Next lends the next contiguous slice of shared memory (zero copy).
func (r *ShmReader) Next() ([]byte, error) {
	if len(r.cur) > 0 {
		b := r.cur
		r.cur = nil
		return b, nil
	}
	if r.eof {
		return nil, io.EOF
	}
	c, err := r.stream.Recv()
	if err == io.EOF {
		r.eof = true
		return nil, io.EOF
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return r.region.Mem[c.Offset : c.Offset+c.Len], nil
}

// Read copies from shared memory into p (one copy).
func (r *ShmReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if len(r.cur) == 0 {
			b, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return n, err
			}
			r.cur = b
		}
		k := copy(p[n:], r.cur)
		r.cur = r.cur[k:]
		n += k
	}
	if n == 0 && r.eof {
		return 0, io.EOF
	}
	return n, nil
}

func (r *ShmReader) Close() error { r.cancel(); return nil }
