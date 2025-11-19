// Copyright 2025 Google LLC
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

package credentials

import (
	"context"
	"os"
	"testing"
)

func TestNewCredentialsFromJSON_SafeTypes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		file     string
		credType CredentialsType
		wantErr  bool
	}{
		{
			name:     "ServiceAccount",
			file:     "../internal/testdata/sa.json",
			credType: ServiceAccount,
			wantErr:  false,
		},
		{
			name:     "UserCredentials",
			file:     "../internal/testdata/user.json",
			credType: UserCredentials,
			wantErr:  false,
		},
		{
			name:     "ServiceAccount_Mismatch",
			file:     "../internal/testdata/user.json",
			credType: ServiceAccount,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) = %v", tt.file, err)
			}
			_, err = NewCredentialsFromJSON(ctx, tt.credType, b, &DetectOptions{})
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCredentialsFromJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCredentialsFromFile_SafeTypes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		file     string
		credType CredentialsType
		wantErr  bool
	}{
		{
			name:     "ServiceAccount",
			file:     "../internal/testdata/sa.json",
			credType: ServiceAccount,
			wantErr:  false,
		},
		{
			name:     "UserCredentials",
			file:     "../internal/testdata/user.json",
			credType: UserCredentials,
			wantErr:  false,
		},
		{
			name:     "ServiceAccount_Mismatch",
			file:     "../internal/testdata/user.json",
			credType: ServiceAccount,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCredentialsFromFile(ctx, tt.credType, tt.file, &DetectOptions{})
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCredentialsFromFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCredentialsFromJSON_UnsafeTypes(t *testing.T) {
	ctx := context.Background()
	// These types are "unsafe" in the sense that they require validation,
	// but the loader itself should still work if the type matches.
	// The warning is in the docstring.

	tests := []struct {
		name     string
		file     string
		credType CredentialsType
		wantErr  bool
	}{
		{
			name:     "ExternalAccount",
			file:     "../internal/testdata/exaccount_url.json",
			credType: ExternalAccount,
			wantErr:  false,
		},
		{
			name:     "ImpersonatedServiceAccount",
			file:     "../internal/testdata/imp.json",
			credType: ImpersonatedServiceAccount,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) = %v", tt.file, err)
			}
			_, err = NewCredentialsFromJSON(ctx, tt.credType, b, &DetectOptions{})
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCredentialsFromJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
