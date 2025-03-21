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

//go:build linux

package metadata

import (
	"os"
	"strings"
)

// defaultSystemInfoSuggestsGCE holds a default placeholder value for
// the systemInfoSuggestsGCE check. It is exposed for testing purposes.
var defaultSystemInfoSuggestsGCE = false

func systemInfoSuggestsGCE() bool {
	b, err := os.ReadFile("/sys/class/dmi/id/product_name")
	if err != nil {
		return defaultSystemInfoSuggestsGCE
	}

	name := strings.TrimSpace(string(b))
	return name == "Google" || name == "Google Compute Engine"
}
