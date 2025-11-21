// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package credentials

import (
	"context"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/auth"
)

func TestNewCredentials(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		credType   CredentialsType
		json       []byte // Use raw JSON to test more cases
		file       string // For file-based tests
		wantErr    bool
		wantErrMsg string // New field to check for specific error messages
		wantCreds  bool   // New field to check for non-nil creds on success
	}{
		// Happy Paths
		{
			name:      "ServiceAccount_Success_FromFile",
			credType:  ServiceAccount,
			file:      "../internal/testdata/sa.json",
			wantErr:   false,
			wantCreds: true,
		},
		{
			name:      "UserCredentials_Success_FromJSON",
			credType:  UserCredentials,
			json:      readTestFile(t, "../internal/testdata/user.json"),
			wantErr:   false,
			wantCreds: true,
		},
		{
			name:      "ExternalAccount_Success_FromFile",
			credType:  ExternalAccount,
			file:      "../internal/testdata/exaccount_url.json",
			wantErr:   false,
			wantCreds: true,
		},
		{
			name:      "ImpersonatedServiceAccount_Success_FromJSON",
			credType:  ImpersonatedServiceAccount,
			json:      readTestFile(t, "../internal/testdata/imp.json"),
			wantErr:   false,
			wantCreds: true,
		},
		// Mismatch Errors
		{
			name:       "ServiceAccount_Mismatch_FromFile",
			credType:   ServiceAccount,
			file:       "../internal/testdata/user.json",
			wantErr:    true,
			wantErrMsg: `credentials: expected type "service_account", found "authorized_user"`,
		},
		{
			name:       "UserCredentials_Mismatch_FromJSON",
			credType:   UserCredentials,
			json:       readTestFile(t, "../internal/testdata/sa.json"),
			wantErr:    true,
			wantErrMsg: `credentials: expected type "authorized_user", found "service_account"`,
		},
		// Other Error Cases
		{
			name:       "Error_MalformedJSON",
			credType:   ServiceAccount,
			json:       []byte(`{"type": "service_account",}`), // Invalid JSON with trailing comma
			wantErr:    true,
			wantErrMsg: "invalid character",
		},
		{
			name:       "Error_MissingTypeField_FromJSON",
			credType:   ServiceAccount,
			json:       []byte(`{"project_id": "my-proj"}`),
			wantErr:    true,
			wantErrMsg: `credentials: expected type "service_account", found ""`,
		},
		{
			name:       "Error_EmptyJSON",
			credType:   ServiceAccount,
			json:       []byte{},
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Error_NonExistentFile",
			credType:   ServiceAccount,
			file:       "testdata/nonexistent.json",
			wantErr:    true,
			wantErrMsg: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var creds *auth.Credentials
			var err error

			if tt.file != "" {
				// Test NewCredentialsFromFile
				creds, err = NewCredentialsFromFile(ctx, tt.credType, tt.file, &DetectOptions{})
			} else {
				// Test NewCredentialsFromJSON
				creds, err = NewCredentialsFromJSON(ctx, tt.credType, tt.json, &DetectOptions{})
			}

			if (err != nil) != tt.wantErr {
				t.Fatalf("NewCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("NewCredentials() error = %q, want error containing %q", err.Error(), tt.wantErrMsg)
				}
				return
			}
			if tt.wantCreds && creds == nil {
				t.Error("NewCredentials() creds = nil, want non-nil")
			}
		})
	}
}

func readTestFile(t *testing.T, filename string) []byte {
	t.Helper()
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = %v", filename, err)
	}
	return b
}
