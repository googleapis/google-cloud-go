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

package internal

// This file declares the two types the AFE picker consumes: afeID
// (identity) and afeSnapshot (immutable score inputs). The producer of
// []afeSnapshot — sessionList — lands in a follow-up PR; splitting the
// types out here lets afe_picker.go ship (and be reviewed) as a pure
// function over snapshot slices.

// afeID identifies the AFE (Application Front End) a session is pinned to,
// derived from PeerInfo.ApplicationFrontendId. The zero value is the
// sentinel for "unknown" — used before PeerInfo is populated or when the
// server did not send the bigtable-peer-info header.
type afeID int64

// afeSnapshot is an immutable view of an AFE bucket sufficient for a
// picker to score and pick without needing to hold the producer's lock
// on the hot path.
//
// Pickers receive value-typed snapshots (no pointer into the producer)
// — the roundtrip back into Checkout is by afeID, re-resolved under the
// producer's lock. This keeps every mutable AFE field guarded by the
// documented lock without relying on the picker to remember not to
// dereference.
//
// Cost fields (TransportCost / E2eCost) are PeakEwma nanoseconds as
// float64. The producer converts the same numbers to time.Duration for
// its debug surface; picker cost functions consume the raw float64.
type afeSnapshot struct {
	ID             afeID
	IdleCount      int
	NumOutstanding int     // refCount − IdleCount, ≥ 0
	TransportCost  float64 // PeakEwma nanoseconds; 0 if never updated
	E2eCost        float64 // PeakEwma nanoseconds; 0 if never updated
}
