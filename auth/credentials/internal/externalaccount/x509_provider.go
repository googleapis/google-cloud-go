// Copyright 2024 Google LLC
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

package externalaccount

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"cloud.google.com/go/auth/internal/transport/cert"
)

type x509Provider struct {
}

func (xp *x509Provider) providerType() string {
	return x509ProviderType
}

func (xp *x509Provider) subjectToken(ctx context.Context) (string, error) {
	return "", nil
}

func createX509Client(certificateConfigLocation string) (*http.Client, error) {
	certProvider, err := cert.NewWorkloadX509CertProvider(certificateConfigLocation)
	if err != nil {
		return nil, err
	}
	trans := http.DefaultTransport.(*http.Transport).Clone()

	trans.TLSClientConfig = &tls.Config{
		GetClientCertificate: certProvider,
	}

	// Create client with default settings plus the X509 workload certs
	client := &http.Client{
		Transport: trans,
		Timeout:   30 * time.Second,
	}

	return client, nil
}
