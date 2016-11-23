// Copyright 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package speech

import (
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1beta1"
	longrunningpb "google.golang.org/genproto/googleapis/longrunning"
)

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var _ = io.EOF

type mockSpeechServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockSpeechServer) SyncRecognize(_ context.Context, req *speechpb.SyncRecognizeRequest) (*speechpb.SyncRecognizeResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*speechpb.SyncRecognizeResponse), nil
}

func (s *mockSpeechServer) AsyncRecognize(_ context.Context, req *speechpb.AsyncRecognizeRequest) (*longrunningpb.Operation, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*longrunningpb.Operation), nil
}

func (s *mockSpeechServer) StreamingRecognize(stream speechpb.Speech_StreamingRecognizeServer) error {
	if s.err != nil {
		return s.err
	}

	ch := make(chan error, 2)
	go func() {
		for {
			if req, err := stream.Recv(); err == io.EOF {
				ch <- nil
				return
			} else if err != nil {
				ch <- err
				return
			} else {
				s.reqs = append(s.reqs, req)
			}
		}
	}()
	go func() {
		for _, v := range s.resps {
			if err := stream.Send(v.(*speechpb.StreamingRecognizeResponse)); err != nil {
				ch <- err
				return
			}
		}
		ch <- nil
	}()

	// Doesn't really matter which one we get.
	err := <-ch
	if err2 := <-ch; err == nil {
		err = err2
	}
	return err
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockSpeech mockSpeechServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	speechpb.RegisterSpeechServer(serv, &mockSpeech)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	go serv.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	clientOpt = option.WithGRPCConn(conn)

	os.Exit(m.Run())
}

func TestSpeechSyncRecognizeError(t *testing.T) {
	errCode := codes.Internal
	mockSpeech.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *speechpb.SyncRecognizeRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.SyncRecognize(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSpeechAsyncRecognizeError(t *testing.T) {
	errCode := codes.Internal
	mockSpeech.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *speechpb.AsyncRecognizeRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.AsyncRecognize(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSpeechStreamingRecognizeError(t *testing.T) {
	errCode := codes.Internal
	mockSpeech.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *speechpb.StreamingRecognizeRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	stream, err := c.StreamingRecognize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
