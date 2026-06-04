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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/security/advancedtls"
)

func tlsOptions(caCertFile, clientCertificatePath, clientKeyPath string) (*advancedtls.Options, error) {
	if caCertFile == "" {
		return nil, nil
	}
	clientCerts, err := clientCertificate(clientCertificatePath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert and key: %w", err)
	}
	capool, err := certPool(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load root CA: %w", err)
	}
	options := &advancedtls.Options{
		VerificationType: advancedtls.CertAndHostVerification,
		IdentityOptions: advancedtls.IdentityCertificateOptions{
			Certificates: clientCerts, // mTLS client certificates.
		},
		RootOptions: advancedtls.RootCertificateOptions{
			RootCertificates: capool, // The CA certificate.
		},
	}
	return options, nil
}

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
		return []option.ClientOption{
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}, nil
	}
	tlsOpts, err := tlsOptions(caCertFile, clientCertFile, clientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS options: %w", err)
	}
	if tlsOpts == nil {
		return nil, fmt.Errorf("TLS configuration options are empty; CA certificate is required for TLS connections")
	}
	creds, err := advancedtls.NewClientCreds(tlsOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS credentials: %w", err)
	}

	return []option.ClientOption{
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(creds)),
	}, nil
}
