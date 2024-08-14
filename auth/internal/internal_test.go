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

package internal

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"cloud.google.com/go/compute/metadata"
)

func TestComputeUniverseDomainProvider(t *testing.T) {
	fatalErr := errors.New("fatal error")
	notDefinedError := metadata.NotDefinedError("universe/universe_domain")
	testCases := []struct {
		name    string
		getFunc func(ctx context.Context) (string, error)
		want    string
		wantErr error
	}{
		{
			name: "test error",
			getFunc: func(ctx context.Context) (string, error) {
				return "", fatalErr
			},
			want:    "",
			wantErr: fatalErr,
		},
		{
			name: "test error 404",
			getFunc: func(ctx context.Context) (string, error) {
				return "", notDefinedError
			},
			want:    DefaultUniverseDomain,
			wantErr: nil,
		},
		{
			name: "test valid",
			getFunc: func(ctx context.Context) (string, error) {
				return "example.com", nil
			},
			want:    "example.com",
			wantErr: nil,
		},
	}

	oldGet := httpGetMetadataUniverseDomain
	defer func() {
		httpGetMetadataUniverseDomain = oldGet
	}()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpGetMetadataUniverseDomain = tc.getFunc
			c := ComputeUniverseDomainProvider{}
			got, err := c.GetProperty(context.Background())
			if err != tc.wantErr {
				t.Errorf("got error %v; want error %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}

type fakeClonableTransport struct {
	clone *http.Transport
}

func (t *fakeClonableTransport) Clone() *http.Transport {
	return t.clone
}

func (t *fakeClonableTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

type fakeTransport struct{}

func (t *fakeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func TestDefaultClient(t *testing.T) {
	transportBeforeTest := http.DefaultTransport
	defer func() { http.DefaultTransport = transportBeforeTest }()

	got := DefaultClient()
	if got.Transport == http.DefaultTransport {
		t.Errorf("DefaultClient() = %v, expected a clone of http.DefaultTransport", got)
	}

	cloneTransport := &http.Transport{}
	http.DefaultTransport = &fakeClonableTransport{clone: cloneTransport}
	got = DefaultClient()
	if got.Transport != cloneTransport {
		t.Errorf("DefaultClient() = %v, want %v", got, cloneTransport)
	}

	fakeTransport := &fakeTransport{}
	http.DefaultTransport = fakeTransport
	got = DefaultClient()
	if got.Transport != fakeTransport {
		t.Errorf("DefaultClient() = %v, want %v", got, fakeTransport)
	}
}
