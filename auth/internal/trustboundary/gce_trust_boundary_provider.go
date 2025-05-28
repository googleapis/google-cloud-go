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
	"fmt"
	"net/http"
	"sync"

	"cloud.google.com/go/auth/internal"
)

// GCETrustBoundaryDataProvider fetches and caches TrustBoundaryData
// for the default service account on a GCE instance. It lazily initializes
// a delegate ServiceAccountTrustBoundaryDataProvider.
type GCETrustBoundaryDataProvider struct {
	// universeDomainProvider provides the universe domain and underlying metadata client.
	universeDomainProvider *internal.ComputeUniverseDomainProvider
	// httpClient is the HTTP client used by the delegate provider for its API calls.
	httpClient *http.Client

	// mu protects delegate and initErr during lazy initialization.
	mu sync.Mutex
	// delegate is the lazily initialized inner provider that handles the actual
	// fetching and caching of TrustBoundaryData.
	delegate TrustBoundaryDataProvider
	// initErr stores any error encountered during the one-time delegate initialization attempt.
	initErr error
}

// NewGCETrustBoundaryDataProvider creates a new GCETrustBoundaryDataProvider.
//
// universeDomainProvider is used to fetch the default service account email and
// universe domain from the GCE metadata server.
//
// httpClient is passed to the underlying ServiceAccountTrustBoundaryDataProvider
// for making API calls to the IAM Credentials endpoint.
func NewGCETrustBoundaryDataProvider(universeDomainProvider *internal.ComputeUniverseDomainProvider, httpClient *http.Client) TrustBoundaryDataProvider {
	return &GCETrustBoundaryDataProvider{
		universeDomainProvider: universeDomainProvider,
		httpClient:             httpClient,
	}
}

// GetTrustBoundaryData implements the TrustBoundaryDataProvider interface.
// It performs a one-time lazy initialization of the delegate provider by fetching
// the GCE default service account email and universe domain from the metadata server,
// and then delegates the trust boundary data request.
func (p *GCETrustBoundaryDataProvider) GetTrustBoundaryData(ctx context.Context, accessToken string) (*TrustBoundaryData, error) {
	p.mu.Lock()
	defer p.mu.Unlock() // Ensure mutex is unlocked upon function exit.

	// If initialization has already been attempted (either successfully or failed),
	// use the stored result directly without retrying the metadata server calls.
	if p.delegate != nil {
		return p.delegate.GetTrustBoundaryData(ctx, accessToken)
	}
	if p.initErr != nil {
		// If initialization failed on a previous attempt, do not retry.
		// This assumes that initial failures (e.g., inability to reach GCE metadata server)
		// are likely due to fundamental environment configuration issues that won't resolve
		// automatically, avoiding repeated, costly, and likely futile attempts.
		return nil, p.initErr
	}

	// This is the first call, and initialization hasn't been attempted or failed yet.
	// Perform one-time initialization of the delegate within the lock.

	// Derive the metadata client from the provided universe domain provider.
	mdClient := p.universeDomainProvider.MetadataClient

	// Fetch the default service account email from GCE metadata.
	// Using "default" gets the primary service account associated with the instance.
	saEmail, err := mdClient.EmailWithContext(ctx, "default")
	if err != nil {
		p.initErr = fmt.Errorf("trustboundary: failed to get GCE service account email: %w", err)
		return nil, p.initErr
	}

	// Fetch the universe domain from GCE metadata.
	universeDomain, err := p.universeDomainProvider.GetProperty(ctx)
	if err != nil {
		// If universe domain cannot be determined, it's a critical setup issue for trust boundaries.
		p.initErr = fmt.Errorf("trustboundary: failed to get GCE universe domain: %w", err)
		return nil, p.initErr
	}

	// Create and store the delegate provider.
	p.delegate = NewServiceAccountTrustBoundaryDataProvider(p.httpClient, saEmail, universeDomain)

	// Delegate the current GetTrustBoundaryData call to the newly initialized provider.
	return p.delegate.GetTrustBoundaryData(ctx, accessToken)
}
