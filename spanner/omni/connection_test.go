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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
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
