// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpcreplay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc/status"

	pb "cloud.google.com/go/internal/rpcreplay/proto/rpcreplay"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	spb "google.golang.org/genproto/googleapis/rpc/status"
)

// An entry holds one gRPC action (request, response, etc.).
type entry struct {
	kind     pb.Entry_Kind
	method   string
	msg      message
	refIndex int // index of corresponding request or create-stream
}

func (e1 *entry) equal(e2 *entry) bool {
	if e1 == nil && e2 == nil {
		return true
	}
	if e1 == nil || e2 == nil {
		return false
	}
	return e1.kind == e2.kind &&
		e1.method == e2.method &&
		proto.Equal(e1.msg.msg, e2.msg.msg) &&
		errEqual(e1.msg.err, e2.msg.err) &&
		e1.refIndex == e2.refIndex
}

func errEqual(e1, e2 error) bool {
	if e1 == e2 {
		return true
	}
	s1, ok1 := status.FromError(e1)
	s2, ok2 := status.FromError(e2)
	if !ok1 || !ok2 {
		return false
	}
	return proto.Equal(s1.Proto(), s2.Proto())
}

// message holds either a single proto.Message or an error.
type message struct {
	msg proto.Message
	err error
}

func (m *message) set(msg interface{}, err error) {
	if msg != nil {
		m.msg = msg.(proto.Message)
	}
	m.err = err
}

// File format:
//   header
//   sequence of Entry protos
//
// Header format:
//   magic string
//   a record containing the bytes of the initial state

const magic = "RPCReplay"

func writeHeader(w io.Writer, initial []byte) error {
	if _, err := io.WriteString(w, magic); err != nil {
		return err
	}
	return writeRecord(w, initial)
}

func readHeader(r io.Reader) ([]byte, error) {
	var buf [len(magic)]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		if err == io.EOF {
			err = errors.New("rpcreplay: empty replay file")
		}
		return nil, err
	}
	if string(buf[:]) != magic {
		return nil, errors.New("rpcreplay: not a replay file (does not begin with magic string)")
	}
	bytes, err := readRecord(r)
	if err == io.EOF {
		err = errors.New("rpcreplay: missing initial state")
	}
	return bytes, err
}

func writeEntry(w io.Writer, e *entry) error {
	var m proto.Message
	if e.msg.err != nil && e.msg.err != io.EOF {
		s, ok := status.FromError(e.msg.err)
		if !ok {
			return fmt.Errorf("rpcreplay: error %v is not a Status", e.msg.err)
		}
		m = s.Proto()
	} else {
		m = e.msg.msg
	}
	var a *any.Any
	var err error
	if m != nil {
		a, err = ptypes.MarshalAny(m)
		if err != nil {
			return err
		}
	}
	pe := &pb.Entry{
		Kind:     e.kind,
		Method:   e.method,
		Message:  a,
		IsError:  e.msg.err != nil,
		RefIndex: int32(e.refIndex),
	}
	bytes, err := proto.Marshal(pe)
	if err != nil {
		return err
	}
	return writeRecord(w, bytes)
}

func readEntry(r io.Reader) (*entry, error) {
	buf, err := readRecord(r)
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pe pb.Entry
	if err := proto.Unmarshal(buf, &pe); err != nil {
		return nil, err
	}
	var msg message
	if pe.Message != nil {
		var any ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(pe.Message, &any); err != nil {
			return nil, err
		}
		if pe.IsError {
			msg.err = status.ErrorProto(any.Message.(*spb.Status))
		} else {
			msg.msg = any.Message
		}
	} else if pe.IsError {
		msg.err = io.EOF
	} else {
		return nil, errors.New("rpcreplay: entry with nil message and false is_error")
	}
	return &entry{
		kind:     pe.Kind,
		method:   pe.Method,
		msg:      msg,
		refIndex: int(pe.RefIndex),
	}, nil
}

// A record consists of an unsigned 32-bit little-endian length L followed by L
// bytes.

func writeRecord(w io.Writer, data []byte) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func readRecord(r io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
