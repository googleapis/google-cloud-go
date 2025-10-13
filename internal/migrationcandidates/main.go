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
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
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
	slog.Info("migrationcandidates: invoked", "args", os.Args)
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migrationcandidates: failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrationcandidates: finished successfully")
}

func run(args []string) error {
	bytes, err := os.ReadFile("internal/migrationcandidates/candidates.txt")
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	// Split the file content into lines
	modules := strings.Split(string(bytes), "\n")

Modules:
	for _, module := range modules {
		output, err := exec.Command("git", "log", "--format=%ae", "-n10", module).Output()
		if err != nil {
			return fmt.Errorf("running git log for %v: %w", module, err)
		}
		authors := strings.Split(string(output), "\n")
		for _, author := range authors {
			switch author {
			case "55107282+release-please[bot]@users.noreply.github.com":
				slog.Info("analysis", "module", module, "status", "safe")
				continue Modules
			case "78513119+gcf-owl-bot[bot]@users.noreply.github.com":
				slog.Info("analysis", "module", module, "status", "unsafe")
				continue Modules
			}
		}
		slog.Info("analysis", "module", module, "status", "uncertain")
	}
	return nil
}
