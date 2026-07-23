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

// afeID identifies the AFE a session is pinned to. Zero = unknown.
type afeID int64

// afeSnapshot is a lock-free view of an AFE bucket for the debug surface
// and the picker's input contract.
type afeSnapshot struct {
	ID             afeID
	IdleCount      int
	NumOutstanding int
	TransportCost  float64 // PeakEwma nanoseconds
	E2eCost        float64 // PeakEwma nanoseconds
}
