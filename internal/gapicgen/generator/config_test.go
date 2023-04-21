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

var packagePathExceptions = map[string]bool{
	"cloud.google.com/go/longrunning/autogen":               true,
	"cloud.google.com/go/firestore/apiv1/admin":             true,
	"cloud.google.com/go/recaptchaenterprise/v2/apiv1":      true,
	"cloud.google.com/go/storage/internal/apiv2":            true,
	"cloud.google.com/go/vision/v2/apiv1":                   true,
	"cloud.google.com/go/recaptchaenterprise/v2/apiv1beta1": true,
	"cloud.google.com/go/vision/v2/apiv1p1beta1":            true,
}

// TestMicrogenConfigs validates config entries.
// We expect entries here to have reasonable production settings, whereas
// the low level tooling is more permissive in the face of ambiguity.
//
// TODO: we should be able to do more substantial validation of config entries
// given sufficient setup.  Consider fetching https://github.com/googleapis/googleapis
// to ensure referenced entries are present.
func TestMicrogenConfigs(t *testing.T) {
	for k, entry := range MicrogenGapicConfigs {
		if entry.ImportPath == "" {
			t.Errorf("config %q (#%d) expected non-empty importPath", entry.InputDirectoryPath, k)
		}
		importParts := strings.Split(entry.ImportPath, "/")
		v := importParts[len(importParts)-1]
		if !strings.HasPrefix(v, "apiv") && !apivExceptions[entry.ImportPath] {
			t.Errorf("config %q (#%d) expected the last part of import path %q to start with \"apiv\"", entry.InputDirectoryPath, k, entry.ImportPath)
		}
		if want := entry.Pkg + "/apiv"; !strings.Contains(entry.ImportPath, want) && !packagePathExceptions[entry.ImportPath] {
			t.Errorf("config %q (#%d) want import path to contain %q", entry.ImportPath, k, want)
		}
		if entry.InputDirectoryPath == "" {
			t.Errorf("config %q (#%d) expected non-empty inputDirectoryPath field", entry.ImportPath, k)
		}
		if entry.Pkg == "" {
			t.Errorf("config %q (#%d) expected non-empty pkg field", entry.ImportPath, k)
		}
		if entry.ApiServiceConfigPath == "" {
			t.Errorf("config %q (#%d) expected non-empty apiServiceConfigPath", entry.ImportPath, k)
		}
		if p := entry.ApiServiceConfigPath; p != "" && strings.HasSuffix(p, "gapic.yaml") {
			t.Errorf("config %q (#%d) should not use GAPIC yaml for apiServiceConfigPath", entry.ImportPath, k)
		}
		// Internally, an empty release level means "ga" to the underlying tool, but we
		// want to be explicit in this configuration.
		if entry.ReleaseLevel == "" {
			t.Errorf("config %q (#%d) expected non-empty releaseLevel field", entry.ImportPath, k)
		} else if !allowedReleaseLevels[entry.ReleaseLevel] {
			t.Errorf("config %q (#%d) invalid releaseLevel: %q", entry.ImportPath, k, entry.ReleaseLevel)
		}
	}
}
