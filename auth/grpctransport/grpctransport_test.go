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
	"log"
	"net"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	echo "cloud.google.com/go/auth/grpctransport/testdata"
	"cloud.google.com/go/auth/internal"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
				}),
			},
		},
		{
			name: "has creds with disable options, cred file",
			opts: &Options{
				DisableAuthentication: true,
				DetectOpts: &credentials.DetectOptions{
					CredentialsFile: "abc.123",
				},
			},
		},
		{
			name: "has creds with disable options, cred json",
			opts: &Options{
				DisableAuthentication: true,
				DetectOpts: &credentials.DetectOptions{
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

func TestDial_SkipValidation(t *testing.T) {
	opts := &Options{
		DisableAuthentication: true,
		Credentials: auth.NewCredentials(&auth.CredentialsOptions{
			TokenProvider: &staticTP{tok: &auth.Token{Value: "fakeToken"}},
		}),
	}
	t.Run("invalid opts", func(t *testing.T) {
		if err := opts.validate(); err == nil {
			t.Fatalf("opts.validate() = nil, want error")
		}
	})

	t.Run("skip invalid opts", func(t *testing.T) {
		opts.InternalOptions = &InternalOptions{SkipValidation: true}
		if err := opts.validate(); err != nil {
			t.Fatalf("opts.validate() = %v, want nil", err)
		}
	})
}

func TestOptions_ValidateWithAllowBoundTokenTypes(t *testing.T) {
	tests := []struct {
		name         string
		in           *Options
		hasALTSCreds bool
	}{
		{
			name: "ALTS hard bound tokens not enabled",
			in: &Options{
				InternalOptions: &InternalOptions{},
			},
		},
		{
			name: "ALTS hard bound tokens enabled and no credentials supplied",
			in: &Options{
				InternalOptions: &InternalOptions{
					AllowHardBoundTokens: []string{"ALTS"},
				},
				DetectOpts: &credentials.DetectOptions{},
			},
			hasALTSCreds: true,
		},
		{
			name: "ALTS hard bound tokens enabled and credentials incompatible",
			in: &Options{
				InternalOptions: &InternalOptions{
					AllowHardBoundTokens: []string{"ALTS"},
				},
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: &staticTP{
						tok: token(map[string]interface{}{
							"auth.google.tokenSource": "user",
						}),
					},
				}),
				DetectOpts: &credentials.DetectOptions{},
			},
		},
		{
			name: "ALTS hard bound tokens enabled and credentials compatible",
			in: &Options{
				InternalOptions: &InternalOptions{
					AllowHardBoundTokens: []string{"ALTS"},
				},
				Credentials: auth.NewCredentials(&auth.CredentialsOptions{
					TokenProvider: &staticTP{
						tok: token(map[string]interface{}{
							"auth.google.tokenSource":    "compute-metadata",
							"auth.google.serviceAccount": "default",
						}),
					},
				}),
				DetectOpts: &credentials.DetectOptions{},
			},
			hasALTSCreds: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.validate(); err != nil {
				t.Fatalf("validate() err = %v, want no error", err)
			}
			if got := tt.in.InternalOptions.altsCredentials != nil; got != tt.hasALTSCreds {
				t.Errorf("InternalOptions.altsCredentials != nil is %v, want %v", got, tt.hasALTSCreds)
			}
		})
	}
}

func TestOptions_ResolveDetectOptions(t *testing.T) {
	tests := []struct {
		name string
		in   *Options
		want *credentials.DetectOptions
	}{
		{
			name: "base",
			in: &Options{
				DetectOpts: &credentials.DetectOptions{
					Scopes:          []string{"scope"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
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
				DetectOpts: &credentials.DetectOptions{
					Scopes:          []string{"scope"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
				Scopes:           []string{"scope"},
				CredentialsFile:  "/path/to/a/file",
				UseSelfSignedJWT: true,
			},
		},
		{
			name: "self-signed, with aud",
			in: &Options{
				DetectOpts: &credentials.DetectOptions{
					Audience:        "aud",
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
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
				DetectOpts: &credentials.DetectOptions{
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
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
				DetectOpts: &credentials.DetectOptions{
					Scopes:          []string{"non-default"},
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
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
				DetectOpts: &credentials.DetectOptions{
					Audience:        "non-default",
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
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
				DetectOpts: &credentials.DetectOptions{
					CredentialsFile: "/path/to/a/file",
				},
			},
			want: &credentials.DetectOptions{
				Audience:        "default",
				CredentialsFile: "/path/to/a/file",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.resolveDetectOptions()
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreFields(credentials.DetectOptions{}, "Logger")); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGrpcCredentialsProvider_GetClientUniverseDomain(t *testing.T) {
	nonDefault := "example.com"
	nonDefault2 := "other-example.com"
	tests := []struct {
		name                 string
		clientUniverseDomain string
		envUniverseDomain    string
		want                 string
	}{
		{
			name:                 "default",
			clientUniverseDomain: "",
			want:                 internal.DefaultUniverseDomain,
		},
		{
			name:                 "client option",
			clientUniverseDomain: nonDefault,
			want:                 nonDefault,
		},
		{
			name:                 "env var",
			clientUniverseDomain: "",
			envUniverseDomain:    nonDefault2,
			want:                 nonDefault2,
		},
		{
			name:                 "client option and env var",
			clientUniverseDomain: nonDefault,
			envUniverseDomain:    nonDefault2,
			want:                 nonDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envUniverseDomain != "" {
				t.Setenv(internal.UniverseDomainEnvVar, tt.envUniverseDomain)
			}
			at := &grpcCredentialsProvider{clientUniverseDomain: tt.clientUniverseDomain}
			got := at.getClientUniverseDomain()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGrpcCredentialsProvider_TokenType(t *testing.T) {
	tests := []struct {
		name string
		tok  *auth.Token
		want string
	}{
		{
			name: "type set",
			tok: &auth.Token{
				Value: "token",
				Type:  "Basic",
			},
			want: "Basic token",
		},
		{
			name: "type set",
			tok: &auth.Token{
				Value: "token",
			},
			want: "Bearer token",
		},
	}
	for _, tc := range tests {
		cp := grpcCredentialsProvider{
			creds: &auth.Credentials{
				TokenProvider: &staticTP{tok: tc.tok},
			},
		}
		m, err := cp.GetRequestMetadata(context.Background(), "")
		if err != nil {
			log.Fatalf("cp.GetRequestMetadata() = %v, want nil", err)
		}
		if got := m["authorization"]; got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

func TestNewClient_DetectedServiceAccount(t *testing.T) {
	testQuota := "testquota"
	wantHeader := "bar"
	t.Setenv(internal.QuotaProjectEnvVar, testQuota)
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
			DefaultEndpointTemplate: l.Addr().String(),
		},
		DetectOpts: &credentials.DetectOptions{
			Audience:         l.Addr().String(),
			CredentialsFile:  "../internal/testdata/sa_universe_domain.json",
			UseSelfSignedJWT: true,
		},
		GRPCDialOpts:   []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		UniverseDomain: "example.com", // Also configured in sa_universe_domain.json
	})
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	client := echo.NewEchoerClient(pool)
	if _, err := client.Echo(context.Background(), &echo.EchoRequest{}); err != nil {
		t.Fatalf("client.Echo() = %v", err)
	}
}

func TestGRPCKeyProvider_GetRequestMetadata(t *testing.T) {
	apiKey := "MY_API_KEY"
	reason := "MY_REQUEST_REASON"
	ts := grpcKeyProvider{
		apiKey: apiKey,
		metadata: map[string]string{
			"X-goog-request-reason": reason,
		},
	}
	got, err := ts.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"X-goog-api-key":        ts.apiKey,
		"X-goog-request-reason": reason,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewClient_QuotaPrecedence(t *testing.T) {
	testQuota := "testquotaWins"
	t.Setenv(internal.QuotaProjectEnvVar, "testquotaLoses")
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
		Metadata: map[string]string{quotaProjectHeaderKey: "testquotaWins"},
		InternalOptions: &InternalOptions{
			DefaultEndpointTemplate: l.Addr().String(),
		},
		DetectOpts: &credentials.DetectOptions{
			Audience:         l.Addr().String(),
			CredentialsFile:  "../internal/testdata/sa_universe_domain.json",
			UseSelfSignedJWT: true,
		},
		GRPCDialOpts:   []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		UniverseDomain: "example.com", // Also configured in sa_universe_domain.json
	})
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	client := echo.NewEchoerClient(pool)
	if _, err := client.Echo(context.Background(), &echo.EchoRequest{}); err != nil {
		t.Fatalf("client.Echo() = %v", err)
	}
}

type staticTP struct {
	tok *auth.Token
}

func (tp *staticTP) Token(context.Context) (*auth.Token, error) {
	return tp.tok, nil
}

type fakeEchoService struct {
	Fn func(context.Context, *echo.EchoRequest) (*echo.EchoReply, error)
	echo.UnimplementedEchoerServer
}

func (s *fakeEchoService) Echo(c context.Context, r *echo.EchoRequest) (*echo.EchoReply, error) {
	return s.Fn(c, r)
}
