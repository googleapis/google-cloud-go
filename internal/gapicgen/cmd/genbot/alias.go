// Copyright 2022 Google LLC
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

//go:build !windows
// +build !windows

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/aliasfix"
	"cloud.google.com/go/internal/aliasgen"
	"cloud.google.com/go/internal/gapicgen/execv/gocmd"
	"cloud.google.com/go/internal/gapicgen/git"
)

type aliasConfig struct {
	gocloudDir  string
	genprotoDir string
}

func generateAliases(ctx context.Context, c aliasConfig) error {
	log.Println("creating temp dir")
	tmpDir, err := ioutil.TempDir("", "update-genproto")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("temp dir created at %s\n", tmpDir)
	// Clone repositories if needed.
	git.DeepClone("https://github.com/googleapis/go-genproto", filepath.Join(tmpDir, "genproto"))
	git.DeepClone("https://github.com/googleapis/google-cloud-go", filepath.Join(tmpDir, "gocloud"))

	genprotoDir := deafultDir(filepath.Join(tmpDir, "genproto"), c.genprotoDir)
	gocloudDir := deafultDir(filepath.Join(tmpDir, "gocloud"), c.gocloudDir)
	for k, v := range aliasfix.GenprotoPkgMigration {
		if v.Status != aliasfix.StatusMigrated {
			continue
		}
		log.Printf("Generating aliases for: %s", k)
		gapicSrcDir := filepath.Join(gocloudDir, strings.TrimPrefix(v.ImportPath, "cloud.google.com/go/"))
		genprotoStubsDir := filepath.Join(genprotoDir, strings.TrimPrefix(k, "google.golang.org/genproto/"))

		// Find out what is the latest version of cloud client library
		module, version, err := gocmd.ListModVersion(gapicSrcDir)
		if err != nil {
			return err
		}

		// checkout said version
		if err := checkoutRef(gocloudDir, module, version); err != nil {
			return err
		}
		// Try to upgrade dependency to said version
		if err := gocmd.Get(genprotoDir, module, version); err != nil {
			return err
		}

		if err := aliasgen.Run(gapicSrcDir, genprotoStubsDir); err != nil {
			return err
		}

		// checkout HEAD of cloud client library
		if err := checkoutRef(gocloudDir, "", ""); err != nil {
			return err
		}
	}

	return nil
}

// checkoutRef checks out the ref that is constructed by using the module
// name and version. If the version provided is empty the main branch will be
// checked out.
func checkoutRef(dir string, modName string, version string) error {
	var ref string
	if version == "" {
		ref = "main"
	} else {
		// Transform cloud.google.com/storage/v2 into storage for example
		vPrefix := strings.Split(strings.TrimPrefix(modName, "cloud.google.com/go/"), "/")[0]
		ref = fmt.Sprintf("%s/%s", vPrefix, version)

	}
	return git.CheckoutRef(dir, ref)
}
