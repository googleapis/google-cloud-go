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

package cert

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/googleapis/enterprise-certificate-proxy/client/util"
)

type certConfigs struct {
	Workload *workloadSource `json:"workload"`
}

type workloadSource struct {
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

type certificateConfig struct {
	CertConfigs certConfigs `json:"cert_configs"`
}

// NewWorkloadX509CertProvider creates a certificate source
// that reads a certificate and private key file from the local file system.
// This is intended to be used for workload identity federation.
//
// The configFilePath points to a config file containing relevant parameters
// such as the certificate and key file paths.
// If configFilePath is empty, the client will attempt to load the config from
// a well-known gcloud location.
func NewWorkloadX509CertProvider(configFilePath string) (Provider, error) {
	if configFilePath == "" {
		envFilePath := util.GetConfigFilePathFromEnv()
		if envFilePath != "" {
			configFilePath = envFilePath
		} else {
			configFilePath = util.GetDefaultConfigFilePath()
		}
	}

	certFile, keyFile, err := getCertAndKeyFiles(configFilePath)

	if err != nil {
		return nil, err
	}

	return (&workloadSource{
		CertPath: certFile,
		KeyPath:  keyFile,
	}).getClientCertificate, nil
}

// getClientCertificate attempts to load the certificate and key from the files specified in the
// certificate config.
func (s *workloadSource) getClientCertificate(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(s.CertPath, s.KeyPath)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// getCertAndKeyFiles attempts to read the provided config file and return the certificate and private
// key file paths.
func getCertAndKeyFiles(configFilePath string) (string, string, error) {
	jsonFile, err := os.Open(configFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", errSourceUnavailable
		}
		return "", "", err
	}

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return "", "", err
	}
	var config certificateConfig
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return "", "", err
	}

	if config.CertConfigs.Workload == nil {
		return "", "", errors.New("workload certificate information not found in certificate configuration")
	}

	certFile := config.CertConfigs.Workload.CertPath
	keyFile := config.CertConfigs.Workload.KeyPath

	if certFile == "" {
		err = errors.New("certificate file location could not be found in the certificate configuration")
	}

	if keyFile == "" {
		err = errors.New("key file location could not be fouind in the certificate configuration")
	}

	if err != nil {
		return "", "", err
	}

	return certFile, keyFile, nil
}
