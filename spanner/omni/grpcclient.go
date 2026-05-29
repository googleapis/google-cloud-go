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
	if clientCertificatePath == "" || clientKeyPath == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(clientCertificatePath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert and key: %w", err)
	}
	return []tls.Certificate{cert}, nil
}

type SpannerOmniConfig struct {
	// UsePlainText specifies whether to use plain text for the connection.
	UsePlainText bool
	// CaCertificateFile is the path to the CA certificate file.
	CaCertificateFile string
	// ClientCertificateFile is the path to the client certificate file.
	ClientCertificateFile string
	// ClientKeyFile is the path to the client key file.
	ClientKeyFile string
}

func ConnectionOptions(config *SpannerOmniConfig) ([]option.ClientOption, error) {
	if config.UsePlainText {
		return []option.ClientOption{
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}, nil
	}
	tlsOpts, err := tlsOptions(config.CaCertificateFile, config.ClientCertificateFile, config.ClientKeyFile)
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
