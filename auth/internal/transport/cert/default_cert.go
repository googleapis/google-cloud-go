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

package cert

import (
	"crypto/tls"
	"errors"
	"sync"
)

// defaultCertData holds all the variables pertaining to
// the default certficate source created by DefaultSource.
//
// A singleton model is used to allow the source to be reused
// by the transport layer.
type defaultCertData struct {
	once   sync.Once
	source Source
	err    error
}

var (
	defaultCert defaultCertData
)

// Source is a function that can be passed into crypto/tls.Config.GetClientCertificate.
type Source func(*tls.CertificateRequestInfo) (*tls.Certificate, error)

// errSourceUnavailable is a sentinel error to indicate certificate source is unavailable.
var errSourceUnavailable = errors.New("certificate source is unavailable")

// DefaultSource returns a certificate source using the preferred EnterpriseCertificateProxySource.
// If EnterpriseCertificateProxySource is not available, fall back to the legacy SecureConnectSource.
//
// If neither source is available (due to missing configurations), a nil Source and a nil Error are
// returned to indicate that a default certificate source is unavailable.
func DefaultSource() (Source, error) {
	defaultCert.once.Do(func() {
		defaultCert.source, defaultCert.err = NewEnterpriseCertificateProxySource("")
		if errors.Is(defaultCert.err, errSourceUnavailable) {
			defaultCert.source, defaultCert.err = NewSecureConnectSource("")
			if errors.Is(defaultCert.err, errSourceUnavailable) {
				defaultCert.source, defaultCert.err = nil, nil
			}
		}
	})
	return defaultCert.source, defaultCert.err
}
