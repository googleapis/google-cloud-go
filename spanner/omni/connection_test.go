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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"filippo.io/nistec"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func generateCerts(t *testing.T) (caFile, certFile, keyFile string) {
	t.Helper()

	// Generate CA
	caPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		t.Fatalf("failed to create ca cert: %v", err)
	}

	tempDir := t.TempDir()
	caFile = filepath.Join(tempDir, "ca.pem")
	caOut, err := os.Create(caFile)
	if err != nil {
		t.Fatalf("failed to open ca.pem: %v", err)
	}
	defer caOut.Close()
	if err := pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes}); err != nil {
		t.Fatalf("failed to write ca.pem: %v", err)
	}

	// Generate Client Cert/Key
	clientPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Test Client Org"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(1 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	clientBytes, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientPrivKey.PublicKey, caPrivKey)
	if err != nil {
		t.Fatalf("failed to create client cert: %v", err)
	}

	certFile = filepath.Join(tempDir, "client.pem")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("failed to open client.pem: %v", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: clientBytes}); err != nil {
		t.Fatalf("failed to write client.pem: %v", err)
	}

	keyFile = filepath.Join(tempDir, "client.key")
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("failed to open client.key: %v", err)
	}
	defer keyOut.Close()
	privBytes, err := x509.MarshalECPrivateKey(clientPrivKey)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}); err != nil {
		t.Fatalf("failed to write client.key: %v", err)
	}

	return caFile, certFile, keyFile
}

func TestConnectionOptions(t *testing.T) {
	t.Run("plaintext connection options", func(t *testing.T) {
		opts, err := ConnectionOptions(true, "", "", "")
		if err != nil {
			t.Fatalf("ConnectionOptions() unexpected error: %v", err)
		}
		if len(opts) != 1 {
			t.Errorf("expected 1 connection option, got %v", len(opts))
		}
	})

	t.Run("valid TLS connection options (System Roots)", func(t *testing.T) {
		opts, err := ConnectionOptions(false, "", "", "")
		if err != nil {
			t.Fatalf("ConnectionOptions() unexpected error: %v", err)
		}
		if len(opts) != 1 {
			t.Errorf("expected 1 connection option, got %v", len(opts))
		}
	})

	t.Run("valid TLS connection options (One-way TLS)", func(t *testing.T) {
		caFile, _, _ := generateCerts(t)
		opts, err := ConnectionOptions(false, caFile, "", "")
		if err != nil {
			t.Fatalf("ConnectionOptions() unexpected error: %v", err)
		}
		if len(opts) != 1 {
			t.Errorf("expected 1 connection option, got %v", len(opts))
		}
	})

	t.Run("valid mTLS connection options", func(t *testing.T) {
		caFile, certFile, keyFile := generateCerts(t)
		opts, err := ConnectionOptions(false, caFile, certFile, keyFile)
		if err != nil {
			t.Fatalf("ConnectionOptions() unexpected error: %v", err)
		}
		if len(opts) != 1 {
			t.Errorf("expected 1 connection option, got %v", len(opts))
		}
	})

	t.Run("missing CA cert file returns error", func(t *testing.T) {
		_, err := ConnectionOptions(false, "nonexistent-ca-file.pem", "", "")
		if err == nil {
			t.Fatal("expected error for nonexistent CA cert file")
		}
	})

	t.Run("missing client cert file returns error", func(t *testing.T) {
		caFile, _, keyFile := generateCerts(t)
		_, err := ConnectionOptions(false, caFile, "nonexistent-client-file.pem", keyFile)
		if err == nil {
			t.Fatal("expected error for nonexistent client cert file")
		}
	})

	t.Run("missing client key file returns error", func(t *testing.T) {
		caFile, certFile, _ := generateCerts(t)
		_, err := ConnectionOptions(false, caFile, certFile, "nonexistent-key-file.key")
		if err == nil {
			t.Fatal("expected error for nonexistent client key file")
		}
	})

	t.Run("only client certificate provided returns error", func(t *testing.T) {
		caFile, certFile, _ := generateCerts(t)
		_, err := ConnectionOptions(false, caFile, certFile, "")
		if err == nil {
			t.Fatal("expected error when client key is missing for mTLS")
		}
	})

	t.Run("only client key provided returns error", func(t *testing.T) {
		caFile, _, keyFile := generateCerts(t)
		_, err := ConnectionOptions(false, caFile, "", keyFile)
		if err == nil {
			t.Fatal("expected error when client certificate is missing for mTLS")
		}
	})
}

type mockLoginServer struct {
	handler func(grpc.ServerStream) error
}

func (s *mockLoginServer) Login(stream grpc.ServerStream) error {
	return s.handler(stream)
}

func startMockServer(t *testing.T, handler func(grpc.ServerStream) error) (*bufconn.Listener, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "google.spanner.auth.v1.LoginService",
		HandlerType: (*interface{})(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "Login",
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					return srv.(*mockLoginServer).Login(stream)
				},
				ServerStreams: true,
				ClientStreams: true,
			},
		},
	}, &mockLoginServer{handler: handler})

	go func() {
		_ = server.Serve(lis)
	}()

	cleanup := func() {
		server.Stop()
		_ = lis.Close()
	}

	return lis, cleanup
}

func TestTokenSource_ServerUnsupportedProtocol(t *testing.T) {
	lis, cleanup := startMockServer(t, func(stream grpc.ServerStream) error {
		req := &LoginRequest{}
		if err := stream.RecvMsg(req); err != nil {
			return err
		}
		resp := &LoginResponse{
			Response: &LoginResponse_HandshakeResponse{
				HandshakeResponse: &PasswordAuthenticationHandshakeResponse{
					PasswordAuthenticationProtocol: PasswordAuthenticationProtocol_PASSWORD_AUTHENTICATION_PROTOCOL_UNSPECIFIED,
				},
			},
		}
		return stream.SendMsg(resp)
	})
	defer cleanup()

	ts := NewTokenSource("user", "pass", []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	})

	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error for unsupported authentication protocol")
	}
}

func TestTokenSource_NoAccessTokenInFinalResponse(t *testing.T) {
	username := "admin"
	password := "password"
	argon2Params := &HashParameters_Argon2IdParameters{
		IterationCount: 3,
		MemoryUsage:    64 * 1024,
		Parallelism:    4,
		HashSize:       32,
	}

	lis, cleanup := startMockServer(t, func(stream grpc.ServerStream) error {
		// 1. Recv Handshake
		req1 := &LoginRequest{}
		if err := stream.RecvMsg(req1); err != nil {
			return err
		}
		resp1 := &LoginResponse{
			Response: &LoginResponse_HandshakeResponse{
				HandshakeResponse: &PasswordAuthenticationHandshakeResponse{
					PasswordAuthenticationProtocol: PasswordAuthenticationProtocol_PASSWORD_AUTHENTICATION_PROTOCOL_OPAQUE,
					HashParameters: &HashParameters{
						Parameters: &HashParameters_Argon2IdParameters_{
							Argon2IdParameters: argon2Params,
						},
					},
				},
			},
		}
		if err := stream.SendMsg(resp1); err != nil {
			return err
		}

		// 2. Recv InitialRequest
		req2 := &LoginRequest{}
		if err := stream.RecvMsg(req2); err != nil {
			return err
		}

		// Dummy OPAQUE initial response
		resp2 := &LoginResponse{
			Response: &LoginResponse_OpaqueResponse{
				OpaqueResponse: &OpaqueLoginResponse{
					Response: &OpaqueLoginResponse_InitialResponse{
						InitialResponse: &InitialOpaqueLoginResponse{
							EvaluatedMessage:     make([]byte, 33),
							MaskingNonce:         make([]byte, 32),
							MaskedResponse:       make([]byte, 97),
							ServerNonce:          make([]byte, 32),
							ServerMac:            make([]byte, 32),
							ServerPublicKeyshare: make([]byte, 33),
						},
					},
				},
			},
		}
		if err := stream.SendMsg(resp2); err != nil {
			return err
		}

		// 3. Recv FinalRequest
		req3 := &LoginRequest{}
		if err := stream.RecvMsg(req3); err != nil {
			return err
		}

		// Send final response with nil AccessToken
		resp3 := &LoginResponse{
			AccessToken: nil,
		}
		return stream.SendMsg(resp3)
	})
	defer cleanup()

	ts := NewTokenSource(username, password, []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	})

	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
}

func TestTokenSource_SuccessAndCaching(t *testing.T) {
	username := "admin"
	password := "admin1234"

	// Pre-derive server & client user registration data for OPAQUE
	serverSeed := []byte("server-seed")
	serverPublicKey, serverPrivateKey, err := deriveKeyPair(serverSeed, []byte(diffieHellmanKeyInfo))
	if err != nil {
		t.Fatalf("deriveKeyPair failed: %v", err)
	}

	oprfSeed := []byte("oprf-seed-32-bytes-long-1234567")
	argon2Params := &HashParameters_Argon2IdParameters{
		IterationCount: 3,
		MemoryUsage:    64 * 1024,
		Parallelism:    4,
		HashSize:       32,
	}
	oprfSeedKey, err := expand(oprfSeed, []byte(username+"OprfKey"), 32)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	_, oprfKey, err := deriveKeyPair(oprfSeedKey, []byte("OPAQUE-DeriveKeyPair"))
	if err != nil {
		t.Fatalf("deriveKeyPair failed: %v", err)
	}
	oprf, err := evaluate(t, []byte(password), oprfKey)
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	stretchedOprf, err := stretch(oprf, argon2Params)
	if err != nil {
		t.Fatalf("stretch failed: %v", err)
	}
	randomizedPassword, err := extract(slices.Concat(oprf, stretchedOprf))
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// Client registration store
	envelopeNonce, err := nonce()
	if err != nil {
		t.Fatalf("nonce failed: %v", err)
	}
	maskingKey, err := expand(randomizedPassword, []byte(maskingKeyInfo), hashLength)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	authKey, err := expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(authKeyInfo)), hashLength)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	seed, err := expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(privateKeyInfo)), hashLength)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	clientPublicKey, _, err := deriveKeyPair(seed, []byte(diffieHellmanKeyInfo))
	if err != nil {
		t.Fatalf("deriveKeyPair failed: %v", err)
	}
	authTag := mac(authKey, slices.Concat(envelopeNonce, serverPublicKey, []byte(username)))

	streamCount := 0

	lis, cleanup := startMockServer(t, func(stream grpc.ServerStream) error {
		streamCount++
		// 1. Recv Handshake
		req1 := &LoginRequest{}
		if err := stream.RecvMsg(req1); err != nil {
			return err
		}
		resp1 := &LoginResponse{
			Response: &LoginResponse_HandshakeResponse{
				HandshakeResponse: &PasswordAuthenticationHandshakeResponse{
					PasswordAuthenticationProtocol: PasswordAuthenticationProtocol_PASSWORD_AUTHENTICATION_PROTOCOL_OPAQUE,
					HashParameters: &HashParameters{
						Parameters: &HashParameters_Argon2IdParameters_{
							Argon2IdParameters: argon2Params,
						},
					},
				},
			},
		}
		if err := stream.SendMsg(resp1); err != nil {
			return err
		}

		// 2. Recv InitialRequest
		req2 := &LoginRequest{}
		if err := stream.RecvMsg(req2); err != nil {
			return err
		}
		initReq := req2.GetOpaqueRequest().GetInitialRequest()

		// Server OPAQUE response generation
		evaluatedElement, err := blindEvaluateServer(username, initReq.BlindedMessage, oprfSeed)
		if err != nil {
			return err
		}
		serverNonce, err := nonce()
		if err != nil {
			return err
		}
		maskingNonce, err := nonce()
		if err != nil {
			return err
		}
		serverPublicKeyshare, serverPrivateKeyshare, err := deriveKeyPair(serverNonce, []byte(diffieHellmanKeyInfo))
		if err != nil {
			return err
		}

		credentialResponsePad, err := expand(maskingKey, slices.Concat(maskingNonce, []byte("CredentialResponsePad")), credentialResponsePadLength)
		if err != nil {
			return err
		}
		clearTextEnvelope := slices.Concat(serverPublicKey, envelopeNonce, authTag)
		maskedResponse, err := xorBytes(clearTextEnvelope, credentialResponsePad)
		if err != nil {
			return err
		}

		dh1, err := diffieHellman(serverPrivateKeyshare, initReq.ClientPublicKeyshare)
		if err != nil {
			return err
		}
		dh2, err := diffieHellman(serverPrivateKey, initReq.ClientPublicKeyshare)
		if err != nil {
			return err
		}
		dh3, err := diffieHellman(serverPrivateKeyshare, clientPublicKey)
		if err != nil {
			return err
		}
		inputKeyMaterial := slices.Concat(dh1, dh2, dh3)

		preamble := slices.Concat([]byte("OPAQUEv1-"), []byte(username), initReq.ClientNonce, initReq.ClientPublicKeyshare, serverPublicKey, evaluatedElement, serverNonce, serverPublicKeyshare)
		km2, _, _, err := deriveSharedKeys(inputKeyMaterial, preamble)
		if err != nil {
			return err
		}
		hashedPreamble := sha256Hash(preamble)
		serverMac := mac(km2, hashedPreamble[:])

		resp2 := &LoginResponse{
			Response: &LoginResponse_OpaqueResponse{
				OpaqueResponse: &OpaqueLoginResponse{
					Response: &OpaqueLoginResponse_InitialResponse{
						InitialResponse: &InitialOpaqueLoginResponse{
							EvaluatedMessage:     evaluatedElement,
							MaskingNonce:         maskingNonce,
							MaskedResponse:       maskedResponse,
							ServerNonce:          serverNonce,
							ServerMac:            serverMac,
							ServerPublicKeyshare: serverPublicKeyshare,
						},
					},
				},
			},
		}
		if err := stream.SendMsg(resp2); err != nil {
			return err
		}

		// 3. Recv FinalRequest
		req3 := &LoginRequest{}
		if err := stream.RecvMsg(req3); err != nil {
			return err
		}

		resp3 := &LoginResponse{
			AccessToken: &AccessToken{
				Signature:      []byte("mock-signature"),
				ExpirationTime: timestamppb.New(time.Now().Add(1 * time.Hour)),
			},
		}
		return stream.SendMsg(resp3)
	})
	defer cleanup()

	ts := NewTokenSource(username, password, []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	})

	token1, err := ts.Token()
	if err != nil {
		t.Fatalf("ts.Token() failed: %v", err)
	}
	if token1 == nil || token1.AccessToken == "" {
		t.Fatalf("expected valid token, got nil/empty")
	}
	if streamCount != 1 {
		t.Errorf("expected 1 stream creation, got %d", streamCount)
	}

	// Second call should returned cached token
	token2, err := ts.Token()
	if err != nil {
		t.Fatalf("ts.Token() second call failed: %v", err)
	}
	if token2.AccessToken != token1.AccessToken {
		t.Errorf("expected cached token %s, got %s", token1.AccessToken, token2.AccessToken)
	}
	if streamCount != 1 {
		t.Errorf("expected streamCount to remain 1 due to token caching, got %d", streamCount)
	}
}

func blindEvaluateServer(username string, pubKey, oprfSeed []byte) ([]byte, error) {
	seed, err := expand(oprfSeed, []byte(username+"OprfKey"), 32)
	if err != nil {
		return nil, fmt.Errorf("expand() failed: %v", err)
	}
	_, oprfKey, err := deriveKeyPair(seed, []byte("OPAQUE-DeriveKeyPair"))
	if err != nil {
		return nil, fmt.Errorf("deriveKeyPair() failed: %v", err)
	}
	blindElement, err := nistec.NewP256Point().SetBytes(pubKey)
	if err != nil {
		return nil, fmt.Errorf("SetBytes() failed: %v", err)
	}
	point, err := blindElement.ScalarMult(blindElement, oprfKey)
	if err != nil {
		return nil, fmt.Errorf("ScalarMult() failed: %v", err)
	}
	return point.Bytes(), nil
}
