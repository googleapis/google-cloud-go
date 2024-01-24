package codec

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

// Custom codec for readObject to reduce data copies during download.
// Replaces standard implementation at https://pkg.go.dev/google.golang.org/grpc/encoding/proto

// Name is the name registered for the proto compressor.
const Name = "proto"

// func init() {
// 	encoding.RegisterCodec(ReadObjectCodec{})
// }

// codec is a Codec implementation with protobuf. It is the default codec for gRPC.
type ReadObjectCodec struct{}

func (ReadObjectCodec) Marshal(v any) ([]byte, error) {
	vv, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
	}
	return proto.Marshal(vv)
}

func (ReadObjectCodec) Unmarshal(data []byte, v any) error {
	vv, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	return proto.Unmarshal(data, vv)
}

func (ReadObjectCodec) Name() string {
	return Name
}
