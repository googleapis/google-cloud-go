package storage

import (
	"fmt"

	"github.com/golang/protobuf/proto"
)

const Name = "" // works
// const Name = "codec" // does not work

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
	fmt.Println("helloworld")
	vv, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}

	if err := proto.Unmarshal(data, vv); err != nil {
		return fmt.Errorf("failed to unmarshal: %v\n", err)
	}

	return nil
}

func (ReadObjectCodec) Name() string {
	return Name
}
