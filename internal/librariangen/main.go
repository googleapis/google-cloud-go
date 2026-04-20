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
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/release"
)

const version = "0.1.0"

// main is the entrypoint for the librariangen CLI.
func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("GOOGLE_SDK_GO_LOGGING_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))
	slog.Debug("librariangen: invoked", "args", os.Args)
	if err := run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("librariangen: failed", "error", err)
		os.Exit(1)
	}
	slog.Debug("librariangen: finished successfully")
}

var (
	releaseStageFunc = release.Stage
)

// run executes the appropriate command based on the CLI's invocation arguments.
// The idiomatic structure is `librariangen [command] [flags]`.
func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("librariangen: expected a command")
	}

	// The --version flag is a special case and not a command.
	if args[0] == "--version" {
		fmt.Println(version)
		return nil
	}

	cmd := args[0]
	flags := args[1:]

	if strings.HasPrefix(cmd, "-") {
		return fmt.Errorf("librariangen: command cannot be a flag: %s", cmd)
	}

	switch cmd {
	case "release-stage":
		return handleReleaseStage(ctx, flags)
	default:
		return fmt.Errorf("librariangen: unknown command: %s", cmd)
	}
}

// handleReleaseStage parses flags for the release-stage command and calls the release tool.
func handleReleaseStage(ctx context.Context, args []string) error {
	cfg := &release.Config{}
	releaseFlags := flag.NewFlagSet("release-stage", flag.ContinueOnError)
	releaseFlags.StringVar(&cfg.LibrarianDir, "librarian", "/librarian", "Path to the librarian-tool input directory. Contains release-stage-request.json.")
	releaseFlags.StringVar(&cfg.RepoDir, "repo", "/repo", "Path to the language repository checkout.")
	releaseFlags.StringVar(&cfg.OutputDir, "output", "/output", "Path to the empty directory where librariangen writes its output.")
	if err := releaseFlags.Parse(args); err != nil {
		return fmt.Errorf("librariangen: failed to parse flags: %w", err)
	}
	return releaseStageFunc(ctx, cfg)
}
