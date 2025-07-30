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

package execv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// Run executes a command and logs its output.
func Run(ctx context.Context, args []string, outputDir string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Dir = outputDir // Run commands from the output directory.
	slog.Debug("running command", "command", strings.Join(cmd.Args, " "), "dir", cmd.Dir)

	output, err := cmd.Output()
	if len(output) > 0 {
		slog.Debug("command stdout", "output", string(output))
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// The command ran and exited with a non-zero exit code.
			if len(exitErr.Stderr) > 0 {
				slog.Debug("command stderr", "output", string(exitErr.Stderr))
			}
			return fmt.Errorf("command failed with exit error: %s: %w", exitErr.Stderr, err)
		}
		// Another error occurred (e.g., command not found).
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}
