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
	certificateConfigLocation string
	cachedClient              *http.Client
}

func (xp *x509Provider) providerType() string {
	return x509ProviderType
}

func (xp *x509Provider) subjectToken(ctx context.Context) (string, error) {
	return "", nil
}

func (xp *x509Provider) client() (*http.Client, error) {
	// Create client if it doesn't already exist
	if xp.cachedClient == nil {
		certProvider, err := cert.NewWorkloadX509CertProvider(xp.certificateConfigLocation)
		if err != nil {
			return nil, err
		}

		trans := http.DefaultTransport.(*http.Transport).Clone()

		trans.TLSClientConfig = &tls.Config{
			GetClientCertificate: certProvider,
		}

		// Create client with default settings plus the X509 workload certs
		xp.cachedClient = &http.Client{
			Transport: trans,
			Timeout:   30 * time.Second,
		}
	}
	return xp.cachedClient, nil
}
