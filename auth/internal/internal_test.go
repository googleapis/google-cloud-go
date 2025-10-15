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
	"reflect"
	"testing"

	"cloud.google.com/go/compute/metadata"
)

func TestComputeUniverseDomainProvider(t *testing.T) {
	fatalErr := errors.New("fatal error")
	notDefinedError := metadata.NotDefinedError("universe/universe_domain")
	testCases := []struct {
		name    string
		getFunc func(context.Context, *metadata.Client) (string, error)
		want    string
		wantErr error
	}{
		{
			name: "test error",
			getFunc: func(context.Context, *metadata.Client) (string, error) {
				return "", fatalErr
			},
			want:    "",
			wantErr: fatalErr,
		},
		{
			name: "test error 404",
			getFunc: func(context.Context, *metadata.Client) (string, error) {
				return "", notDefinedError
			},
			want:    DefaultUniverseDomain,
			wantErr: nil,
		},
		{
			name: "test valid",
			getFunc: func(context.Context, *metadata.Client) (string, error) {
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

func TestNewTrustBoundaryData(t *testing.T) {
	tests := []struct {
		name             string
		locations        []string
		encodedLocations string
		wantLocations    []string
		wantEncoded      string
	}{
		{
			name:             "Standard data",
			locations:        []string{"us-central1", "europe-west1"},
			encodedLocations: "0xABC123",
			wantLocations:    []string{"us-central1", "europe-west1"},
			wantEncoded:      "0xABC123",
		},
		{
			name:             "Empty locations, not no-op encoded",
			locations:        []string{},
			encodedLocations: "0xDEF456",
			wantLocations:    []string{},
			wantEncoded:      "0xDEF456",
		},
		{
			name:             "Nil locations, not no-op encoded",
			locations:        nil,
			encodedLocations: "0xGHI789",
			wantLocations:    []string{}, // Expect empty slice, not nil
			wantEncoded:      "0xGHI789",
		},
		{
			name:             "No-op encoded locations",
			locations:        []string{"us-east1"},
			encodedLocations: TrustBoundaryNoOp,
			wantLocations:    []string{"us-east1"},
			wantEncoded:      TrustBoundaryNoOp,
		},
		{
			name:             "Empty string encoded locations",
			locations:        []string{},
			encodedLocations: "",
			wantLocations:    []string{},
			wantEncoded:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := NewTrustBoundaryData(tt.locations, tt.encodedLocations)

			if got := data.EncodedLocations; got != tt.wantEncoded {
				t.Errorf("NewTrustBoundaryData().EncodedLocations = %q, want %q", got, tt.wantEncoded)
			}

			gotLocations := data.Locations
			if !reflect.DeepEqual(gotLocations, tt.wantLocations) {
				t.Errorf("NewTrustBoundaryData().Locations = %v, want %v", gotLocations, tt.wantLocations)
			}
		})
	}
}

func TestNewNoOpTrustBoundaryData(t *testing.T) {
	data := NewNoOpTrustBoundaryData()

	if data == nil {
		t.Fatal("NewNoOpTrustBoundaryData() returned nil")
	}

	if got := data.EncodedLocations; got != TrustBoundaryNoOp {
		t.Errorf("NewNoOpTrustBoundaryData().EncodedLocations = %q, want %q", got, TrustBoundaryNoOp)
	}
}

func TestTrustBoundaryHeader(t *testing.T) {
	tests := []struct {
		name        string
		tbd         TrustBoundaryData
		wantValue   string
		wantPresent bool
	}{
		{
			name:        "empty data",
			tbd:         TrustBoundaryData{},
			wantValue:   "",
			wantPresent: false,
		},
		{
			name:        "no-op data",
			tbd:         *NewNoOpTrustBoundaryData(),
			wantValue:   "",
			wantPresent: true,
		},
		{
			name:        "regular data",
			tbd:         *NewTrustBoundaryData(nil, "some-encoded-locations"),
			wantValue:   "some-encoded-locations",
			wantPresent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotPresent := tt.tbd.TrustBoundaryHeader()
			if gotValue != tt.wantValue {
				t.Errorf("TrustBoundaryHeader() gotValue = %v, want %v", gotValue, tt.wantValue)
			}
			if gotPresent != tt.wantPresent {
				t.Errorf("TrustBoundaryHeader() gotPresent = %v, want %v", gotPresent, tt.wantPresent)
			}
		})
	}
}
