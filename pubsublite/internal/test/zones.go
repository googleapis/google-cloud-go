// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package test

import (
	"math/rand"
	"time"

	"cloud.google.com/go/internal/testutil"
)

var (
	supportedZones = []string{
		"us-central1-a",
		"us-central1-b",
		"us-central1-c",
		"us-east1-b",
		"us-east1-c",
		"europe-west1-b",
		"europe-west1-d",
		"asia-east1-a",
		"asia-east1-c",
		"asia-southeast1-a",
		"asia-southeast1-c",
	}

	rng *rand.Rand
)

func init() {
	rng = testutil.NewRand(time.Now())
}

// RandomLiteZone chooses a random Pub/Sub Lite zone for integration tests.
func RandomLiteZone() string {
	return supportedZones[rng.Intn(len(supportedZones))]
}
