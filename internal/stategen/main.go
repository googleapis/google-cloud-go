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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// main is the entrypoint for the stategen tool, which updates a Librarian state.yaml
// file to include the specified modules. The first argument is a path to the repository
// root; all subsequent arguments are module names.
func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("GOOGLE_SDK_GO_LOGGING_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))
	slog.Info("stategen: invoked", "args", os.Args)
	if err := run(os.Args[1:]); err != nil {
		slog.Error("stategen: failed", "error", err)
		os.Exit(1)
	}
	slog.Info("stategen: finished successfully")
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("stategen: expected a root directory and at least one module")
	}
	repoRoot := args[0]

	postProcessorConfigPath := filepath.Join(repoRoot, "internal/postprocessor/config.yaml")
	ppc, err := loadPostProcessorConfig(postProcessorConfigPath)
	if err != nil {
		return err
	}

	googleapisCommit, err := findLatestGoogleapisCommit()
	if err != nil {
		return err
	}

	stateFilePath := filepath.Join(repoRoot, ".librarian/state.yaml")
	state, err := parseLibrarianState(stateFilePath)
	if err != nil {
		return err
	}

	for _, moduleName := range args[1:] {
		if stateContainsModule(state, moduleName) {
			slog.Info("skipping existing module", "module", moduleName)
			continue
		}
		moduleRoot := filepath.Join(repoRoot, moduleName)
		if err := prepareModule(moduleRoot); err != nil {
			return fmt.Errorf("preparing module %s: %w", moduleName, err)
		}
		if err := addModule(repoRoot, ppc, state, moduleName, googleapisCommit); err != nil {
			return err
		}
		if err := cleanupLegacyConfigs(repoRoot, moduleName); err != nil {
			return err
		}
	}

	return saveLibrarianState(stateFilePath, state)
}
