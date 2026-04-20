/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"testing"
	"time"
)

func TestEndpointLatencyRegistryKeysByOperationUID(t *testing.T) {
	clearEndpointLatencyRegistry()
	defer clearEndpointLatencyRegistry()

	endpointLatencyRegistryRecordLatency(7, false, "server-a:443", 25*time.Millisecond)

	if !endpointLatencyRegistryHasScore(7, false, "server-a:443") {
		t.Fatal("expected score for recorded operation/address")
	}
	if endpointLatencyRegistryHasScore(8, false, "server-a:443") {
		t.Fatal("expected different operation UID to have no score")
	}
	if endpointLatencyRegistryHasScore(7, true, "server-a:443") {
		t.Fatal("expected preferLeader to remain part of the key")
	}
}
