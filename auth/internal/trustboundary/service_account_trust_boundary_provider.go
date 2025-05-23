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

package trustboundary

import (
	"context"
	"net/http"
	"sync"
)

// ServiceAccountTrustBoundaryDataProvider fetches and caches TrustBoundaryData for a service account.
// It implements the auth.TrustBoundaryDataProvider interface.
// This provider is thread-safe for concurrent access to its cached data.
type ServiceAccountTrustBoundaryDataProvider struct {
	client              *http.Client
	serviceAccountEmail string
	universeDomain      string

	mu   sync.Mutex // Protects 'data' for concurrent access.
	data *TrustBoundaryData
	// Note: We don't store 'err' here because LookupServiceAccountTrustBoundary
	// already handles fallback logic. We only propagate an error if no data
	// (neither new nor cached) can be returned.
}

// NewServiceAccountTrustBoundaryDataProvider creates a new ServiceAccountTrustBoundaryDataProvider.
func NewServiceAccountTrustBoundaryDataProvider(client *http.Client, saEmail, universeDomain string) TrustBoundaryDataProvider {
	return &ServiceAccountTrustBoundaryDataProvider{
		client:              client,
		serviceAccountEmail: saEmail,
		universeDomain:      universeDomain,
	}
}

// GetTrustBoundaryData implements the auth.TrustBoundaryDataProvider interface.
// It retrieves the trust boundary data for the configured service account, utilizing caching and fallback.
func (p *ServiceAccountTrustBoundaryDataProvider) GetTrustBoundaryData(ctx context.Context) (*TrustBoundaryData, error) {
	// Acquire lock to safely read cached data before potentially making a network call.
	p.mu.Lock()
	cachedData := p.data
	p.mu.Unlock()

	// LookupServiceAccountTrustBoundary handles the core logic of fetching new data,
	// applying no-op rules, and falling back to cachedData if necessary, returning nil error on successful fallback.
	newData, err := LookupServiceAccountTrustBoundary(ctx, p.client, p.serviceAccountEmail, cachedData, p.universeDomain)

	// Re-acquire lock to safely update the cache and return.
	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {
		// If an error occurred, it means LookupServiceAccountTrustBoundary could not return
		// any valid data (neither new nor cached).
		return nil, err
	}

	// No error means valid data was successfully fetched or confirmed (e.g., no-op, or valid fallback).
	// Update the cache with this result.
	p.data = newData
	return newData, nil
}
