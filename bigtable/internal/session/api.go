// Copyright 2026 Google LLC
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

// Package session hosts the vRPC-over-session data-plane API surface
// for the bigtable Go client. The interfaces defined here describe a
// proto-native alternative to the classic gRPC TableAPI: methods take
// and return *SessionReadRow{Request,Response} /
// *SessionMutateRow{Request,Response} instead of bigtable.Row.
//
// This file establishes the shape only; implementations land in
// follow-up changes. Nothing in this package imports the top-level
// bigtable package.
package session

import (
	"context"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"go.opentelemetry.io/otel/metric"
)

// TableAPI is the per-resource, proto-native data-plane API.
// Implementations route ReadRow over a read session pool and MutateRow
// over a separate write session pool; callers do not see the
// distinction. Pools open lazily on first call, so a resource that
// only ever reads never pays for a write pool.
type TableAPI interface {
	ReadRow(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error)
	MutateRow(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error)

	// Close releases this resource's underlying read + write session
	// pools. Independent from Client.Close — closing a single
	// resource does not close the shared channel pool.
	Close() error
}

// Client owns the underlying gRPC channel pool + stub and vends
// per-resource TableAPI instances. Does NOT cache — callers are
// responsible for caching per-resource entries so repeat Opens reuse
// the same underlying pools.
type Client interface {
	// OpenSessionTable returns a TableAPI for a standard table,
	// identified by the leaf table name (e.g. "my-table"). Full
	// resource composition happens inside the implementation.
	OpenSessionTable(tableName string) TableAPI

	// OpenAuthorizedView returns a TableAPI for a specific
	// authorized view under table.
	OpenAuthorizedView(table, view string) TableAPI

	// OpenMaterializedView returns a read-only TableAPI for a
	// materialized view. MutateRow on the returned handle errors.
	OpenMaterializedView(view string) TableAPI

	// MeterProvider exposes the OpenTelemetry meter provider the
	// Client was constructed with — same instance the bigtable
	// client uses for its own metrics, so callers can register
	// additional instruments against the same provider.
	MeterProvider() metric.MeterProvider

	// AddSessionLoadListener registers a listener invoked every time
	// the server-driven client configuration reports a new
	// session-load ratio (0.0 = classic-only, 1.0 = session-only).
	// Returns an unregister thunk.
	AddSessionLoadListener(func(load float64)) func()

	// Close closes the underlying channel pool.
	//
	// Callers should close every vended TableAPI first — this
	// tears down the shared channel pool, and vended tables can no
	// longer issue cleanup RPCs (e.g., session deletion) once the pool
	// is gone. Any TableAPI still open at the time of this call
	// becomes unusable.
	Close() error
}
