// Copyright 2020 Google LLC
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

package generator

import (
	"bytes"
	"fmt"
	"testing"
)

var allowedReleaseLevels = map[string]bool{
	"alpha": true,
	"beta":  true,
	"ga":    true,
}

// TestMicrogenConfigs validates config entries.
// We expect entries here to have reasonable production settings, whereas
// the low level tooling is more permissive in the face of ambiguity.
//
// TODO: we should be able to do more substantial validation of config entries
// given sufficient setup.  Consider fetching https://github.com/googleapis/googleapis
// to ensure referenced entries are present.
func TestMicrogenConfigs(t *testing.T) {
	for k, entry := range microgenGapicConfigs {
		// Collect all the problems for a given entry and report them once.
		var errors []error

		if entry.importPath == "" {
			errors = append(errors, fmt.Errorf("expected non-empty import field"))
		}
		if entry.inputDirectoryPath == "" {
			errors = append(errors, fmt.Errorf("expected non-empty inputDirectoryPath field"))
		}
		if entry.pkg == "" {
			errors = append(errors, fmt.Errorf("expected non-empty pkg field"))
		}
		// TODO: Consider if we want to allow this at a later point in time.  If this
		// isn't supplied the config is technically valid, but the generated library
		// won't include features such as retry policies.
		if entry.gRPCServiceConfigPath == "" {
			errors = append(errors, fmt.Errorf("expected non-empty gRPCServiceConfigPath"))
		}
		if entry.apiServiceConfigPath == "" {
			errors = append(errors, fmt.Errorf("expected non-empty apiServiceConfigPath"))
		}
		// Internally, an empty release level means "ga" to the underlying tool, but we
		// want to be explicit in this configuration.
		if entry.releaseLevel == "" {
			errors = append(errors, fmt.Errorf("expected non-empty releaseLevel field"))
		} else if _, ok := allowedReleaseLevels[entry.releaseLevel]; !ok {
			errors = append(errors, fmt.Errorf("invalid release level: %s", entry.releaseLevel))
		}

		if len(errors) > 0 {
			var b bytes.Buffer
			fmt.Fprintf(&b, "errors with entry #%d (importPath: %s)\n", k, entry.importPath)
			for _, v := range errors {
				fmt.Fprintf(&b, "\t%v\n", v)
			}
			t.Errorf(b.String())
		}
	}
}
