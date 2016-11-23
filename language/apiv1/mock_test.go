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

package language

import (
	languagepb "google.golang.org/genproto/googleapis/cloud/language/v1"
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

type mockLanguageServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockLanguageServer) AnalyzeSentiment(_ context.Context, req *languagepb.AnalyzeSentimentRequest) (*languagepb.AnalyzeSentimentResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnalyzeSentimentResponse), nil
}

func (s *mockLanguageServer) AnalyzeEntities(_ context.Context, req *languagepb.AnalyzeEntitiesRequest) (*languagepb.AnalyzeEntitiesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnalyzeEntitiesResponse), nil
}

func (s *mockLanguageServer) AnalyzeSyntax(_ context.Context, req *languagepb.AnalyzeSyntaxRequest) (*languagepb.AnalyzeSyntaxResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnalyzeSyntaxResponse), nil
}

func (s *mockLanguageServer) AnnotateText(_ context.Context, req *languagepb.AnnotateTextRequest) (*languagepb.AnnotateTextResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnnotateTextResponse), nil
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockLanguage mockLanguageServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	languagepb.RegisterLanguageServiceServer(serv, &mockLanguage)

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

func TestLanguageServiceAnalyzeSentimentError(t *testing.T) {
	errCode := codes.Internal
	mockLanguage.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *languagepb.AnalyzeSentimentRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.AnalyzeSentiment(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestLanguageServiceAnalyzeEntitiesError(t *testing.T) {
	errCode := codes.Internal
	mockLanguage.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *languagepb.AnalyzeEntitiesRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.AnalyzeEntities(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestLanguageServiceAnalyzeSyntaxError(t *testing.T) {
	errCode := codes.Internal
	mockLanguage.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *languagepb.AnalyzeSyntaxRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.AnalyzeSyntax(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestLanguageServiceAnnotateTextError(t *testing.T) {
	errCode := codes.Internal
	mockLanguage.err = grpc.Errorf(errCode, "test error")

	c, err := NewClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *languagepb.AnnotateTextRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.AnnotateText(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
