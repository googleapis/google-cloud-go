// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "testing"

func TestLoadConfig(t *testing.T) {
	p := &postProcessor{
		googleCloudDir: "../..",
		googleapisDir:  googleapisDir,
	}
	if err := p.loadConfig(); err != nil {
		t.Fatal(err)
	}

	li := p.config.GoogleapisToImportPath["google/cloud/alloydb/connectors/v1"]
	if got, want := li.ImportPath, "cloud.google.com/go/alloydb/connectors/apiv1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := li.ServiceConfig, "connectors_v1.yaml"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := li.RelPath, "/alloydb/connectors/apiv1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := li.ReleaseLevel, "preview"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
