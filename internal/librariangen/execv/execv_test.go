// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package execv

import (
	"context"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   bool
		wantInErr string
	}{
		{
			name:    "valid command",
			args:    []string{"echo", "hello"},
			wantErr: false,
		},
		{
			name:    "invalid command",
			args:    []string{"command-that-does-not-exist"},
			wantErr: true,
		},
		{
			name:    "command with non-zero exit",
			args:    []string{"sh", "-c", "exit 1"},
			wantErr: true,
		},
		{
			name:      "command with stderr output",
			args:      []string{"sh", "-c", "echo 'test error' >&2; exit 1"},
			wantErr:   true,
			wantInErr: "test error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(context.Background(), tt.args, ".")
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantInErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
					t.Errorf("Run() error = %v, want substring %q", err, tt.wantInErr)
				}
			}
		})
	}
}
