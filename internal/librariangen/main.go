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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"cloud.google.com/go/internal/postprocessor/librarian/librariangen/generate"
)

const version = "0.1.0"

// main is the entrypoint for the librariangen CLI.
func main() {
	slog.Info("librariangen invoked", "args", os.Args)
	if err := run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("librariangen failed", "error", err)
		os.Exit(1)
	}
	slog.Info("librariangen finished successfully")
}

var generateFunc = generate.Generate

// run executes the appropriate command based on the CLI's invocation arguments.
func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		// Check for a version flag if no other args are present.
		for _, arg := range os.Args[1:] {
			if arg == "--version" {
				fmt.Println(version)
				return nil
			}
		}
		return fmt.Errorf("expected a command")
	}

	// Separate command and flags. The command is the first non-flag argument.
	var cmd string
	var flags []string
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			cmd = arg
			flags = append(args[:i], args[i+1:]...)
			break
		}
	}

	// Handle --version flag even if other flags are present.
	for _, arg := range args {
		if arg == "--version" {
			fmt.Println(version)
			return nil
		}
	}

	if cmd == "" {
		// This case handles when all arguments are flags, which is an error.
		// Or when there are no arguments.
		return fmt.Errorf("no command specified")
	}

	switch cmd {
	case "generate":
		return handleGenerate(ctx, flags)
	case "configure":
		slog.Warn("configure command is not yet implemented")
		return nil
	case "build":
		slog.Warn("build command is not yet implemented")
		return nil
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

// handleGenerate parses flags for the generate command and calls the generator.
func handleGenerate(ctx context.Context, args []string) error {
	cfg := &generate.Config{}
	generateFlags := flag.NewFlagSet("generate", flag.ExitOnError)
	generateFlags.StringVar(&cfg.LibrarianDir, "librarian", "/librarian", "Path to the librarian-tool input directory. Contains generate-request.json.")
	generateFlags.StringVar(&cfg.InputDir, "input", "/input", "Path to the .librarian/generator-input directory from the language repository.")
	generateFlags.StringVar(&cfg.OutputDir, "output", "/output", "Path to the empty directory where librariangen writes its output.")
	generateFlags.StringVar(&cfg.SourceDir, "source", "/source", "Path to a complete checkout of the googleapis repository.")
	generateFlags.BoolVar(&cfg.EnablePostProcessor, "enable-post-processor", false, "Enable the post-processor. This should always be true in production.")
	if err := generateFlags.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return generateFunc(ctx, cfg)
}
