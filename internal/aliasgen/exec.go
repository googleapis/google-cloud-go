// Copyright 2022 Google LLC
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

package aliasgen

import (
	"log"
	"os/exec"
)

var isTest bool

func goImports(dir string) error {
	log.Println("Running `goimports`")
	cmd := exec.Command("goimports", "-w", ".")
	cmd.Dir = dir
	return cmd.Run()
}

func goModTidy(dir string) error {
	if isTest {
		return nil
	}
	log.Println("Running `go mod tidy`")
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	return cmd.Run()
}
