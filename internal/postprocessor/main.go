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

package main

import (
	"flag"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/internal/postprocessor/execv/gocmd"
)

func main() {
	srcRoot := flag.String("src", "owl-bot-staging/src/", "Path to owl-bot-staging directory")
	dstRoot := flag.String("dst", "", "Path to clients")
	flag.Parse()

	srcPrefix := *srcRoot
	dstPrefix := *dstRoot

	log.Println("srcPrefix set to", srcPrefix)
	log.Println("dstPrefix set to", dstPrefix)

	if err := run(srcPrefix, dstPrefix); err != nil {
		log.Fatal(err)
	}

	// TODO: delete owl-bot-staging file

	log.Println("Files copied and formatted from owl-bot-staging to libraries.")
}

func run(srcPrefix, dstPrefix string) error {
	filepath.WalkDir(srcPrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		dstPath := filepath.Join(dstPrefix, strings.TrimPrefix(path, srcPrefix))

		if err := copyFiles(path, dstPath); err != nil {
			return err
		}

		return nil
	})

	if err := gocmd.ModTidyAll("."); err != nil {
		return err
	}

	if err := gocmd.Vet("."); err != nil {
		return err
	}

	return nil
}

func copyFiles(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	return nil
}
