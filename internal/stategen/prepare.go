// Copyright 2025 Google LLC
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
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// prepareModule runs a series of cleanup and formatting operations on a given
// module's source code, equivalent to the prepare_module.sh script.
func prepareModule(moduleRoot string) error {
	slog.Info("preparing module", "path", moduleRoot)

	// chmod -x $(find . -name '*.go')
	if err := removeGoExecutePermissions(moduleRoot); err != nil {
		return fmt.Errorf("removing execute permissions: %w", err)
	}

	// goimports -w .
	if err := runGoImports(moduleRoot); err != nil {
		return fmt.Errorf("running goimports: %w", err)
	}

	// for file in $(find . -name '*_grpc.pb.go') ...
	if err := removeIgnoredGrpcFiles(moduleRoot); err != nil {
		return fmt.Errorf("removing ignored gRPC files: %w", err)
	}

	return nil
}

func removeGoExecutePermissions(moduleRoot string) error {
	return filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		// Check if any execute bit is set
		if perm&0111 != 0 {
			slog.Debug("removing execute permission", "file", path)
			// Set permissions to current permissions minus execute bits
			if err := os.Chmod(path, perm&^0111); err != nil {
				return fmt.Errorf("chmod failed for %s: %w", path, err)
			}
		}
		return nil
	})
}

func runGoImports(moduleRoot string) error {
	cmd := exec.Command("goimports", "-w", ".")
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	slog.Debug("running goimports", "dir", moduleRoot)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("goimports failed: %w\n%s", err, stderr.String())
	}
	return nil
}

func removeIgnoredGrpcFiles(moduleRoot string) error {
	const buildIgnore = "//go:build ignore"
	return filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), "_grpc.pb.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		if strings.Contains(string(content), buildIgnore) {
			slog.Info("deleting ignored gRPC file", "file", path)
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("deleting %s: %w", path, err)
			}
		}
		return nil
	})
}
