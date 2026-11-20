package udsthin

import (
	"fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	pb "poc/gothin/udsthin/gcscorepb"
)

// rawChunk is a ReadChunk decoded WITHOUT copying the payload: `parts`
// reference the gRPC receive buffers directly (the same trick the native SDK's
// bytesCodecV2 uses). The caller must call release() once consumed.
type rawChunk struct {
	parts   [][]byte
	meta    *pb.ObjectMeta
	release func()
}

// rawCodec decodes ReadChunk/ShmChunk responses by walking the protobuf wire
// format over the mem.BufferSlice; everything else goes through the stock
// proto codec.
type rawCodec struct{ base encoding.CodecV2 }

const rawCodecName = "proto" // registered as the default so stubs pick it up

func (c rawCodec) Name() string { return rawCodecName }

func (c rawCodec) Marshal(v any) (mem.BufferSlice, error) {
	if wr, ok := v.(*pb.WriteRequest); ok {
		if d, ok := wr.Msg.(*pb.WriteRequest_Data); ok {
			if len(d.Data) == 0 {
				return nil, nil
			}
			// Field 2 (bytes data), wire type 2 (BytesType) -> tag = 0x12.
			hdr := make([]byte, 0, 10)
			hdr = protowire.AppendTag(hdr, 2, protowire.BytesType)
			hdr = protowire.AppendVarint(hdr, uint64(len(d.Data)))
			return mem.BufferSlice{
				mem.SliceBuffer(hdr),
				mem.SliceBuffer(d.Data),
			}, nil
		}
	}
	return c.base.Marshal(v)
}

func unmarshalSingleBuffer(b []byte, rc *rawChunk) (bool, error) {
	rc.parts = rc.parts[:0]
	rc.meta = nil
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return false, nil
		}
		b = b[n:]
		switch typ {
		case protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return false, nil
			}
			b = b[m:]
			switch num {
			case 1:
				rc.parts = append(rc.parts, v)
			case 2, 3:
				meta := &pb.ObjectMeta{}
				if err := proto.Unmarshal(v, meta); err != nil {
					return true, err
				}
				rc.meta = meta
			}
		case protowire.VarintType:
			_, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return false, nil
			}
			b = b[m:]
		case protowire.Fixed32Type:
			_, m := protowire.ConsumeFixed32(b)
			if m < 0 {
				return false, nil
			}
			b = b[m:]
		case protowire.Fixed64Type:
			_, m := protowire.ConsumeFixed64(b)
			if m < 0 {
				return false, nil
			}
			b = b[m:]
		default:
			return false, nil
		}
	}
	return true, nil
}

func (c rawCodec) Unmarshal(data mem.BufferSlice, v any) error {
	rc, ok := v.(*rawChunk)
	if !ok {
		return c.base.Unmarshal(data, v)
	}
	// Keep the buffers alive beyond this call; released by the reader.
	data.Ref()
	rc.release = data.Free
	if len(data) == 1 {
		if ok, err := unmarshalSingleBuffer(data[0].ReadOnlyData(), rc); ok {
			return err
		}
	}
	// Flatten view over the slice without copying.
	type seg struct {
		b []byte
	}
	segs := make([]seg, 0, len(data))
	total := 0
	for _, buf := range data {
		b := buf.ReadOnlyData()
		segs = append(segs, seg{b})
		total += len(b)
	}
	// cursor over segments
	si, so := 0, 0
	readByte := func() (byte, bool) {
		for si < len(segs) {
			if so < len(segs[si].b) {
				x := segs[si].b[so]
				so++
				return x, true
			}
			si++
			so = 0
		}
		return 0, false
	}
	readVarint := func() (uint64, error) {
		var x uint64
		var s uint
		for i := 0; i < 10; i++ {
			b, ok := readByte()
			if !ok {
				return 0, fmt.Errorf("rawcodec: truncated varint")
			}
			x |= uint64(b&0x7f) << s
			if b < 0x80 {
				return x, nil
			}
			s += 7
		}
		return 0, fmt.Errorf("rawcodec: varint overflow")
	}
	// take n bytes as zero or more sub-slices of the underlying buffers
	take := func(n int) ([][]byte, error) {
		var out [][]byte
		for n > 0 {
			if si >= len(segs) {
				return nil, fmt.Errorf("rawcodec: truncated bytes field")
			}
			avail := len(segs[si].b) - so
			if avail == 0 {
				si++
				so = 0
				continue
			}
			k := min(avail, n)
			out = append(out, segs[si].b[so:so+k])
			so += k
			n -= k
		}
		return out, nil
	}
	rc.parts = rc.parts[:0]
	rc.meta = nil
	for {
		tag, err := readVarint()
		if err != nil {
			if si >= len(segs) {
				break // clean EOF
			}
			return err
		}
		if si >= len(segs) && so == 0 && tag == 0 {
			break
		}
		num, typ := protowire.DecodeTag(tag)
		switch typ {
		case protowire.BytesType:
			l, err := readVarint()
			if err != nil {
				return err
			}
			parts, err := take(int(l))
			if err != nil {
				return err
			}
			switch num {
			case 1: // data (repeated bytes) — alias, no copy
				rc.parts = append(rc.parts, parts...)
			case 2, 3: // meta (ReadChunk uses 2, ShmChunk uses 3)
				buf := make([]byte, 0, l)
				for _, p := range parts {
					buf = append(buf, p...)
				}
				m := &pb.ObjectMeta{}
				if err := proto.Unmarshal(buf, m); err != nil {
					return err
				}
				rc.meta = m
			default:
				// unknown bytes field: skipped
			}
		case protowire.VarintType:
			if _, err := readVarint(); err != nil {
				return err
			}
		case protowire.Fixed32Type:
			take(4)
		case protowire.Fixed64Type:
			take(8)
		default:
			return fmt.Errorf("rawcodec: unsupported wire type %d", typ)
		}
		if si >= len(segs) || (si == len(segs)-1 && so >= len(segs[si].b)) {
			break
		}
	}
	return nil
}

func init() {
	base := encoding.GetCodecV2("proto")
	encoding.RegisterCodecV2(rawCodec{base: base})
}
