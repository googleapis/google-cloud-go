// Copyright 2023 Google LLC
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

package grpctransport

import (
	"context"
	"errors"
	"net"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/detect"
	echo "cloud.google.com/go/auth/grpctransport/testdata"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestCheckDirectPathEndPoint(t *testing.T) {
	for _, testcase := range []struct {
		name     string
		endpoint string
		want     bool
	}{
		{
			name:     "empty endpoint are disallowed",
			endpoint: "",
			want:     false,
		},
		{
			name:     "dns schemes are allowed",
			endpoint: "dns:///foo",
			want:     true,
		},
		{
			name:     "host without no prefix are allowed",
			endpoint: "foo",
			want:     true,
		},
		{
			name:     "host with port are allowed",
			endpoint: "foo:1234",
			want:     true,
		},
		{
			name:     "non-dns schemes are disallowed",
			endpoint: "https://foo",
			want:     false,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if got := checkDirectPathEndPoint(testcase.endpoint); got != testcase.want {
				t.Fatalf("got %v, want %v", got, testcase.want)
			}
		})
	}
}

func TestDial_FailsValidation(t *testing.T) {
	tests := []struct {
		name string
		opts *Options
	}{
		{
			name: "missing options",
		},
		{
			name: "has creds with disable options, tp",
			opts: &Options{
				DisableAuthentication: true,
				TokenProvider:         staticTP("fakeToken"),
			},
		},
		{
			name: "has creds with disable options, cred file",
			opts: &Options{
				DisableAuthentication: true,
				DetectOpts: &detect.Options{
					CredentialsFile: "abc.123",
				},
			},
		},
		{
			name: "has creds with disable options, cred json",
			opts: &Options{
				DisableAuthentication: true,
				DetectOpts: &detect.Options{
					CredentialsJSON: []byte(`{"foo":"bar"}`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Dial(context.Background(), false, tt.opts)
			if err == nil {
				t.Fatal("NewClient() = _, nil, want error")
			}
		})
	}
}

func TestOptions_ResolveDetectOptions(t *testing.T) {
	tests := []struct {
		name string
		in   *Options
		want *detect.Options
	}{
		{
			name: "base",
			in: &Options{
				DetectOpts: &detect.Options{
					Scopes:          []string{"scope"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:          []string{"scope"},
				CredentialsFile: "/path/to/a/file",
			},
		},
		{
			name: "self-signed, with scope",
			in: &Options{
				InternalOptions: &InternalOptions{
					EnableJWTWithScope: true,
				},
				DetectOpts: &detect.Options{
					Scopes:          []string{"scope"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:           []string{"scope"},
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "self-signed, with aud",
			in: &Options{
				DetectOpts: &detect.Options{
					Audience:        "aud",
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Audience:         "aud",
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "use default scopes",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultScopes:   []string{"default"},
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:          []string{"default"},
				CredentialsFile: "/path/to/a/file",
			},
		},
		{
			name: "don't use default scopes, scope provided",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultScopes:   []string{"default"},
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					Scopes:          []string{"non-default"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Scopes:          []string{"non-default"},
				CredentialsFile: "/path/to/a/file",
			},
		},
		{
			name: "don't use default scopes, aud provided",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultScopes:   []string{"default"},
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					Audience:        "non-default",
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Audience:         "non-default",
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "use default aud",
			in: &Options{
				InternalOptions: &InternalOptions{
					DefaultAudience: "default",
				},
				DetectOpts: &detect.Options{
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &detect.Options{
				Audience:        "default",
				CredentialsFile: "/path/to/a/file",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.resolveDetectOptions()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewClient_DetectedServiceAccount(t *testing.T) {
	testQuota := "testquota"
	wantHeader := "bar"
	t.Setenv("GOOGLE_CLOUD_QUOTA_PROJECT", testQuota)
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	gsrv := grpc.NewServer()
	defer gsrv.Stop()
	echo.RegisterEchoerServer(gsrv, &fakeEchoService{
		Fn: func(ctx context.Context, _ *echo.EchoRequest) (*echo.EchoReply, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				t.Error("unable to extract metadata")
				return nil, errors.New("oops")
			}
			if got := md.Get("authorization"); len(got) != 1 {
				t.Errorf(`got "", want an auth token`)
			}
			if got := md.Get("Foo"); len(got) != 1 || got[0] != wantHeader {
				t.Errorf("got %q, want %q", got, wantHeader)
			}
			if got := md.Get(quotaProjectHeaderKey); len(got) != 1 || got[0] != testQuota {
				t.Errorf("got %q, want %q", got, testQuota)
			}
			return &echo.EchoReply{}, nil
		},
	})
	go func() {
		if err := gsrv.Serve(l); err != nil {
			panic(err)
		}
	}()

	pool, err := Dial(context.Background(), false, &Options{
		Metadata: map[string]string{"Foo": wantHeader},
		InternalOptions: &InternalOptions{
			DefaultEndpoint: l.Addr().String(),
		},
		DetectOpts: &detect.Options{
			Audience:         l.Addr().String(),
			CredentialsFile:  "../internal/testdata/sa.json",
			UseSelfSignedJWT: true,
		},
		GRPCDialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	client := echo.NewEchoerClient(pool)
	if _, err := client.Echo(context.Background(), &echo.EchoRequest{}); err != nil {
		t.Fatalf("client.Echo() = %v", err)
	}
}

type staticTP string

func (tp staticTP) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{
		Value: string(tp),
	}, nil
}

type fakeEchoService struct {
	Fn func(context.Context, *echo.EchoRequest) (*echo.EchoReply, error)
	echo.UnimplementedEchoerServer
}

func (s *fakeEchoService) Echo(c context.Context, r *echo.EchoRequest) (*echo.EchoReply, error) {
	return s.Fn(c, r)
}
