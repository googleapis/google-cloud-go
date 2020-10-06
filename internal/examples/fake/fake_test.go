// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fake

import (
	"context"
	"net"
	"testing"

	translate "cloud.google.com/go/translate/apiv3"
	"google.golang.org/api/option"
	translatepb "google.golang.org/genproto/googleapis/cloud/translate/v3"
	"google.golang.org/grpc"
)

// fakeTranslationServer respresents a fake gRPC server where all of the methods
// are unimplemented except TranslateText which is explicitly overridden.
type fakeTranslationServer struct {
	translatepb.UnimplementedTranslationServiceServer
}

func (f *fakeTranslationServer) TranslateText(ctx context.Context, req *translatepb.TranslateTextRequest) (*translatepb.TranslateTextResponse, error) {
	resp := &translatepb.TranslateTextResponse{
		Translations: []*translatepb.Translation{
			{TranslatedText: "Hello World"},
		},
	}
	return resp, nil
}

func TestTranslateTextWithConcreteClient(t *testing.T) {
	ctx := context.Background()

	// Setup the fake server.
	fakeTranslationServer := &fakeTranslationServer{}
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	gsrv := grpc.NewServer()
	translatepb.RegisterTranslationServiceServer(gsrv, fakeTranslationServer)
	fakeServerAddr := l.Addr().String()
	go func() {
		if err := gsrv.Serve(l); err != nil {
			panic(err)
		}
	}()

	// Create a client.
	client, err := translate.NewTranslationClient(ctx,
		option.WithEndpoint(fakeServerAddr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Run the test.
	text, err := TranslateTextWithConcreteClient(client, "Hola Mundo", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello World" {
		t.Fatalf("got %q, want Hello World", text)
	}
}
