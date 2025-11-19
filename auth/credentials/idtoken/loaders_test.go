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

package idtoken

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/auth/credentials"
)

func TestNewCredentialsFromJSON_SafeTypes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		file     string
		credType credentials.CredentialsType
		wantErr  bool
	}{
		{
			name:     "ServiceAccount",
			file:     "../../internal/testdata/sa.json",
			credType: credentials.ServiceAccount,
			wantErr:  false,
		},
		{
			name:     "ServiceAccount_Mismatch",
			file:     "../../internal/testdata/user.json",
			credType: credentials.ServiceAccount,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) = %v", tt.file, err)
			}
			_, err = NewCredentialsFromJSON(ctx, tt.credType, b, &Options{Audience: "aud"})
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
		credType credentials.CredentialsType
		wantErr  bool
	}{
		{
			name:     "ServiceAccount",
			file:     "../../internal/testdata/sa.json",
			credType: credentials.ServiceAccount,
			wantErr:  false,
		},
		{
			name:     "ServiceAccount_Mismatch",
			file:     "../../internal/testdata/user.json",
			credType: credentials.ServiceAccount,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCredentialsFromFile(ctx, tt.credType, tt.file, &Options{Audience: "aud"})
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCredentialsFromFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
