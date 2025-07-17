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

func TestTrustBoundaryData(t *testing.T) {
	tests := []struct {
		name        string
		tbd         *TrustBoundaryData
		wantIsNoOp  bool
		wantIsEmpty bool
	}{
		{
			name:        "nil",
			tbd:         nil,
			wantIsNoOp:  false,
			wantIsEmpty: true,
		},
		{
			name: "empty",
			tbd: &TrustBoundaryData{
				EncodedLocations: "",
			},
			wantIsNoOp:  false,
			wantIsEmpty: true,
		},
		{
			name: "no-op",
			tbd: &TrustBoundaryData{
				EncodedLocations: TrustBoundaryNoOp,
			},
			wantIsNoOp:  true,
			wantIsEmpty: false,
		},
		{
			name: "with locations",
			tbd: &TrustBoundaryData{
				EncodedLocations: "some-locations",
			},
			wantIsNoOp:  false,
			wantIsEmpty: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tbd.IsNoOp(); got != tt.wantIsNoOp {
				t.Errorf("IsNoOp() = %v, want %v", got, tt.wantIsNoOp)
			}
			if got := tt.tbd.IsEmpty(); got != tt.wantIsEmpty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.wantIsEmpty)
			}
		})
	}
}

func TestNewTrustBoundaryData(t *testing.T) {
	tests := []struct {
		name             string
		locations        []string
		encodedLocations string
		wantLocations    []string
		wantEncoded      string
		wantIsNoOp       bool
		wantIsEmpty      bool
	}{
		{
			name:             "Standard data",
			locations:        []string{"us-central1", "europe-west1"},
			encodedLocations: "0xABC123",
			wantLocations:    []string{"us-central1", "europe-west1"},
			wantEncoded:      "0xABC123",
			wantIsNoOp:       false,
			wantIsEmpty:      false,
		},
		{
			name:             "Empty locations, not no-op encoded",
			locations:        []string{},
			encodedLocations: "0xDEF456",
			wantLocations:    []string{},
			wantEncoded:      "0xDEF456",
			wantIsNoOp:       false,
			wantIsEmpty:      false,
		},
		{
			name:             "Nil locations, not no-op encoded",
			locations:        nil,
			encodedLocations: "0xGHI789",
			wantLocations:    []string{}, // Expect empty slice, not nil
			wantEncoded:      "0xGHI789",
			wantIsNoOp:       false,
			wantIsEmpty:      false,
		},
		{
			name:             "No-op encoded locations",
			locations:        []string{"us-east1"},
			encodedLocations: TrustBoundaryNoOp,
			wantLocations:    []string{"us-east1"},
			wantEncoded:      TrustBoundaryNoOp,
			wantIsNoOp:       true,
			wantIsEmpty:      false,
		},
		{
			name:             "Empty string encoded locations",
			locations:        []string{},
			encodedLocations: "",
			wantLocations:    []string{},
			wantEncoded:      "",
			wantIsNoOp:       false,
			wantIsEmpty:      true,
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

			if got := data.IsNoOp(); got != tt.wantIsNoOp {
				t.Errorf("NewTrustBoundaryData(...).IsNoOp() = %v, want %v", got, tt.wantIsNoOp)
			}
			if got := data.IsEmpty(); got != tt.wantIsEmpty {
				t.Errorf("NewTrustBoundaryData(...).IsEmpty() = %v, want %v", got, tt.wantIsEmpty)
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

	if !data.IsNoOp() {
		t.Errorf("NewNoOpTrustBoundaryData().IsNoOp() = false, want true")
	}
	if data.IsEmpty() {
		t.Errorf("NewNoOpTrustBoundaryData().IsEmpty() = true, want false")
	}
}

func TestData_Methods_NilReceiver(t *testing.T) {
	var data *TrustBoundaryData = nil

	if got := (*TrustBoundaryData)(data).IsNoOp(); got {
		t.Errorf("nil TrustBoundaryData.IsNoOp() = true, want false")
	}
	if got := (*TrustBoundaryData)(data).IsEmpty(); !got {
		t.Errorf("nil TrustBoundaryData.IsEmpty() = false, want true")
	}
}

func TestTrustBoundaryHeader(t *testing.T) {
	tests := []struct {
		name        string
		tbd         *TrustBoundaryData
		wantValue   string
		wantPresent bool
	}{
		{
			name:        "nil data",
			tbd:         nil,
			wantValue:   "",
			wantPresent: false,
		},
		{
			name:        "empty data",
			tbd:         NewTrustBoundaryData(nil, ""),
			wantValue:   "",
			wantPresent: false,
		},
		{
			name:        "no-op data",
			tbd:         NewTrustBoundaryData(nil, TrustBoundaryNoOp),
			wantValue:   "",
			wantPresent: true,
		},
		{
			name:        "regular data",
			tbd:         NewTrustBoundaryData(nil, "some-encoded-locations"),
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
