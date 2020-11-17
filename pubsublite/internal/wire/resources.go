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

package wire

import (
	"fmt"
	"strings"
)

// ValidateZone verifies that the `input` string has the format of a valid
// Google Cloud zone. An example zone is "europe-west1-b".
// See https://cloud.google.com/compute/docs/regions-zones for more information.
func ValidateZone(input string) error {
	parts := strings.Split(input, "-")
	if len(parts) != 3 {
		return fmt.Errorf("pubsublite: invalid zone %q", input)
	}
	return nil
}

// ValidateRegion verifies that the `input` string has the format of a valid
// Google Cloud region. An example region is "europe-west1".
// See https://cloud.google.com/compute/docs/regions-zones for more information.
func ValidateRegion(input string) error {
	parts := strings.Split(input, "-")
	if len(parts) != 2 {
		return fmt.Errorf("pubsublite: invalid region %q", input)
	}
	return nil
}

type topicPartition struct {
	Path      string
	Partition int
}

type subscriptionPartition struct {
	Path      string
	Partition int
}
