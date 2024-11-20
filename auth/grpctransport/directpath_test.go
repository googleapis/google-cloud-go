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

package grpctransport

import (
	"bytes"
	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/compute/metadata"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestIsTokenProviderDirectPathCompatible(t *testing.T) {
	for _, tt := range []struct {
		name string
		tp   auth.TokenProvider
		opts *Options
		want bool
	}{
		{
			name: "empty TokenProvider",
			opts: &Options{},
		},
		{
			name: "err TokenProvider.Token",
			tp:   &errTP{},
			opts: &Options{},
			want: false,
		},
		{
			name: "EnableNonDefaultSAForDirectPath",
			tp:   &staticTP{tok: &auth.Token{Value: "fakeToken"}},
			opts: &Options{
				InternalOptions: &InternalOptions{
					EnableNonDefaultSAForDirectPath: true,
				},
			},
			want: true,
		},
		{
			name: "non-compute token source",
			tp:   &staticTP{tok: token(map[string]interface{}{"auth.google.tokenSource": "NOT-compute-metadata"})},
			opts: &Options{},
			want: false,
		},
		{
			name: "compute-metadata but non default SA",
			tp: &staticTP{
				tok: token(map[string]interface{}{
					"auth.google.tokenSource":    "compute-metadata",
					"auth.google.serviceAccount": "NON-default",
				}),
			},
			opts: &Options{},
			want: false,
		},
		{
			name: "non-default service account",
			tp:   &staticTP{tok: token(map[string]interface{}{"auth.google.serviceAccount": "NOT-default"})},
			opts: &Options{},
			want: false,
		},
		{
			name: "default service account on compute",
			tp: &staticTP{
				tok: token(map[string]interface{}{
					"auth.google.tokenSource":    "compute-metadata",
					"auth.google.serviceAccount": "default",
				}),
			},
			opts: &Options{},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTokenProviderDirectPathCompatible(tt.tp, tt.opts); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDirectPathBoundTokenEnabled(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts *InternalOptions
		want bool
	}{
		{
			name: "empty list",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{},
			},
		},
		{
			name: "nil list",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{},
			},
		},
		{
			name: "list does not contain ALTS",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{"MTLS_S2A"},
			},
		},
		{
			name: "list only contains ALTS",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{"ALTS"},
			},
			want: true,
		},
		{
			name: "list contains ALTS and others",
			opts: &InternalOptions{
				AllowHardBoundTokens: []string{"ALTS", "MTLS_S2A"},
			},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDirectPathBoundTokenEnabled(tt.opts); got != tt.want {
				t.Fatalf("isDirectPathBoundTokenEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

type errTP struct {
}

func (tp *errTP) Token(context.Context) (*auth.Token, error) {
	return nil, errors.New("error fetching Token")
}

func token(metadata map[string]interface{}) *auth.Token {
	tok := &auth.Token{Value: "fakeToken"}
	tok.Metadata = metadata
	return tok
}

func TestLogDirectPathMisconfigDirectPathNotSet(t *testing.T) {
	opts := &Options{InternalOptions: &InternalOptions{}}
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"
	creds, err := credentials.DetectDefault(opts.resolveDetectOptions())
	if err != nil {
		t.Fatalf("failed to create creds")
	}

	logDirectPathMisconfig(endpoint, creds, opts)

	wantedLog := "DirectPath is disabled. To enable, please set the EnableDirectPath option along with the EnableDirectPathXds option."
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestLogDirectPathMisconfigWrongCredential(t *testing.T) {
	opts := &Options{InternalOptions: &InternalOptions{
		EnableDirectPathXds: true,
		EnableDirectPath:    true,
	}}
	opts.InternalOptions.EnableDirectPath = true
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"
	creds := &auth.Credentials{}

	logDirectPathMisconfig(endpoint, creds, opts)

	wantedLog := "DirectPath is disabled. Please make sure the token source is fetched from GCE metadata server and the default service account is used."
	if !strings.Contains(logOutput.String(), wantedLog) {
		t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
	}
}

func TestLogDirectPathMisconfigNotOnGCE(t *testing.T) {
	opts := &Options{InternalOptions: &InternalOptions{}}
	opts.InternalOptions.EnableDirectPath = true
	opts.InternalOptions.EnableDirectPathXds = true

	var logOutput bytes.Buffer
	opts.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

	endpoint := "abc.googleapis.com"

	creds, err := credentials.DetectDefault(opts.resolveDetectOptions())
	if err != nil {
		t.Fatalf("failed to create creds")
	}

	logDirectPathMisconfig(endpoint, creds, opts)

	if !metadata.OnGCE() {
		wantedLog := "DirectPath is disabled. DirectPath is only available in a GCE environment."
		if !strings.Contains(logOutput.String(), wantedLog) {
			t.Fatalf("got: %v, want: %v", logOutput.String(), wantedLog)
		}
	}
}
