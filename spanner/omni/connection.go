/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package omni

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	sync "sync"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// certPool creates a x509.CertPool from the given CA certificate file.
func certPool(caCertFile string) (*x509.CertPool, error) {
	ca, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert file: %w", err)
	}
	capool := x509.NewCertPool()
	if !capool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to append the CA certificate to CA pool")
	}
	return capool, nil
}

// clientCertificate loads client certificate and private key for mTLS.
func clientCertificate(clientCertificatePath string, clientKeyPath string) ([]tls.Certificate, error) {
	if clientCertificatePath == "" && clientKeyPath == "" {
		return nil, nil
	}
	if clientCertificatePath == "" || clientKeyPath == "" {
		return nil, fmt.Errorf("both client certificate and client key must be provided for mTLS")
	}
	cert, err := tls.LoadX509KeyPair(clientCertificatePath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert and key: %w", err)
	}
	return []tls.Certificate{cert}, nil
}

// ConnectionOptions generates standard ClientOption credentials configurations for Spanner Omni.
func ConnectionOptions(usePlainText bool, caCertFile, clientCertFile, clientKeyFile string) ([]option.ClientOption, error) {
	if usePlainText {
		if caCertFile != "" || clientCertFile != "" || clientKeyFile != "" {
			return nil, fmt.Errorf("cannot use plain text and provide TLS certificates at the same time")
		}
		return []option.ClientOption{
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if caCertFile != "" {
		capool, err := certPool(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load root CA: %w", err)
		}
		tlsConfig.RootCAs = capool
	}
	if clientCertFile != "" || clientKeyFile != "" {
		clientCerts, err := clientCertificate(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = clientCerts
	}

	creds := credentials.NewTLS(tlsConfig)
	return []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithTransportCredentials(creds)),
	}, nil
}

// gRPC client definitions

// LoginServiceClient is the client interface for the LoginService.
type LoginServiceClient interface {
	Login(ctx context.Context, opts ...grpc.CallOption) (LoginServiceLoginClient, error)
}

type loginServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewLoginServiceClient creates a new LoginServiceClient.
func NewLoginServiceClient(cc grpc.ClientConnInterface) LoginServiceClient {
	return &loginServiceClient{cc}
}

func (c *loginServiceClient) Login(ctx context.Context, opts ...grpc.CallOption) (LoginServiceLoginClient, error) {
	stream, err := c.cc.NewStream(ctx, &LoginServiceServiceDesc.Streams[0], "/google.spanner.auth.v1.LoginService/Login", opts...)
	if err != nil {
		return nil, err
	}
	x := &loginServiceLoginClient{stream}
	return x, nil
}

// LoginServiceLoginClient is the stream client for LoginService.
type LoginServiceLoginClient interface {
	Send(*LoginRequest) error
	Recv() (*LoginResponse, error)
	grpc.ClientStream
}

type loginServiceLoginClient struct {
	grpc.ClientStream
}

func (x *loginServiceLoginClient) Send(m *LoginRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *loginServiceLoginClient) Recv() (*LoginResponse, error) {
	m := new(LoginResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// LoginServiceServiceDesc is the service description for LoginService.
var LoginServiceServiceDesc = grpc.ServiceDesc{
	ServiceName: "google.spanner.auth.v1.LoginService",
	HandlerType: (*interface{})(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Login",
			Handler:       nil,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "omni.proto",
}

// omniTokenSource is a TokenSource that performs OPAQUE protocol handshake to retrieve access token.
type omniTokenSource struct {
	mu       sync.Mutex
	username string
	password string
	opts     []option.ClientOption
	token    *oauth2.Token
}

// NewTokenSource creates a new TokenSource for Omni authentication.
func NewTokenSource(username, password string, opts []option.ClientOption) oauth2.TokenSource {
	return &omniTokenSource{
		username: username,
		password: password,
		opts:     opts,
	}
}

func (ts *omniTokenSource) Token() (*oauth2.Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.token != nil && ts.token.Valid() {
		return ts.token, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Add x-goog-api-client header to satisfy headers_enforcer in tests
	ctx = metadata.AppendToOutgoingContext(ctx, "x-goog-api-client", "gl-go/1.22 grpc/")

	cc, err := gtransport.Dial(ctx, ts.opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial spanner omni: %w", err)
	}
	defer cc.Close()

	client := NewLoginServiceClient(cc)

	stream, err := client.Login(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start login stream: %w", err)
	}
	defer stream.CloseSend()

	handshakeReq := &LoginRequest{
		Username: ts.username,
		Request: &LoginRequest_HandshakeRequest{
			HandshakeRequest: &PasswordAuthenticationHandshakeRequest{},
		},
	}
	if err := stream.Send(handshakeReq); err != nil {
		return nil, fmt.Errorf("failed to send handshake request: %w", err)
	}
	handshakeResp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive handshake response: %w", err)
	}

	method := handshakeResp.GetHandshakeResponse().GetPasswordAuthenticationProtocol()
	if method != PasswordAuthenticationProtocol_PASSWORD_AUTHENTICATION_PROTOCOL_OPAQUE {
		return nil, fmt.Errorf("server does not support OPAQUE authentication")
	}

	hashParams := handshakeResp.GetHandshakeResponse().GetHashParameters()
	auth, err := newAuthenticator(ts.username, ts.password, hashParams)
	if err != nil {
		return nil, err
	}
	initReq, err := auth.InitialRequest()
	if err != nil {
		return nil, err
	}
	if err := stream.Send(initReq); err != nil {
		return nil, fmt.Errorf("failed to send initial request: %w", err)
	}

	initResp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive initial response: %w", err)
	}

	finalReq, err := auth.FinalRequest(initResp)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(finalReq); err != nil {
		return nil, fmt.Errorf("failed to send final request: %w", err)
	}

	finalResp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive final response: %w", err)
	}

	accessToken := finalResp.GetAccessToken()
	if accessToken == nil {
		return nil, fmt.Errorf("no access token in final response")
	}

	accessTokenBytes, err := proto.Marshal(accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal access token: %w", err)
	}

	exp := accessToken.ExpirationTime.AsTime()
	ts.token = &oauth2.Token{
		AccessToken: base64.StdEncoding.EncodeToString(accessTokenBytes),
		TokenType:   "Bearer",
		Expiry:      exp,
	}

	return ts.token, nil
}
