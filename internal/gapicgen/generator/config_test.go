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
	"strings"
	"testing"
)

var allowedReleaseLevels = map[string]bool{
	"alpha": true,
	"beta":  true,
	"ga":    true,
}

var apivExceptions = map[string]bool{
	"cloud.google.com/go/longrunning/autogen":   true,
	"cloud.google.com/go/firestore/apiv1/admin": true,
	"cloud.google.com/go/cloudbuild/apiv1/v2":   true,
	"cloud.google.com/go/monitoring/apiv3/v2":   true,
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
		if entry.importPath == "" {
			t.Errorf("config %q (#%d) expected non-empty importPath", entry.inputDirectoryPath, k)
		}
		importParts := strings.Split(entry.importPath, "/")
		v := importParts[len(importParts)-1]
		if !strings.HasPrefix(v, "apiv") && !apivExceptions[entry.importPath] {
			t.Errorf("config %q (#%d) expected the last part of import path %q to start with \"apiv\"", entry.inputDirectoryPath, k, entry.importPath)
		}
		if entry.inputDirectoryPath == "" {
			t.Errorf("config %q (#%d) expected non-empty inputDirectoryPath field", entry.importPath, k)
		}
		if entry.pkg == "" {
			t.Errorf("config %q (#%d) expected non-empty pkg field", entry.importPath, k)
		}
		// TODO: Consider if we want to allow this at a later point in time.  If this
		// isn't supplied the config is technically valid, but the generated library
		// won't include features such as retry policies.
		if entry.gRPCServiceConfigPath == "" {
			t.Errorf("config %q (#%d) expected non-empty gRPCServiceConfigPath", entry.importPath, k)
		}
		if entry.apiServiceConfigPath == "" {
			t.Errorf("config %q (#%d) expected non-empty apiServiceConfigPath", entry.importPath, k)
		}
		// Internally, an empty release level means "ga" to the underlying tool, but we
		// want to be explicit in this configuration.
		if entry.releaseLevel == "" {
			t.Errorf("config %q (#%d) expected non-empty releaseLevel field", entry.importPath, k)
		} else if !allowedReleaseLevels[entry.releaseLevel] {
			t.Errorf("config %q (#%d) invalid releaseLevel: %q", entry.importPath, k, entry.releaseLevel)
		}
	}
}
