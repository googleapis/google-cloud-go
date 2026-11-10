// Copyright 2026 Google LLC
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

package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetGitCredsOwnerOnlyPermissions verifies the git credential file written
// by setGitCredsToDir — which contains the GitHub PAT in plaintext — is created
// with owner-only permissions (0600). On shared hosts with world-traversable
// home directories (0755 default on most distros), a 0644 credentials file
// lets any local user read the PAT.
//
// The test runs hermetically against a temp home directory; the developer's
// real ~/.git-credentials is never touched.
func TestSetGitCredsOwnerOnlyPermissions(t *testing.T) {
	home := t.TempDir()

	err := setGitCredsToDir(home, "Test User", "test@example.com", "testuser", "ghp_FAKE_TOKEN")
	if err != nil {
		t.Skipf("setGitCredsToDir requires git on PATH: %v", err)
	}

	p := filepath.Join(home, ".git-credentials")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read credentials file: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat credentials file: %v", err)
	}
	perm := fi.Mode().Perm()

	if !strings.Contains(string(data), "ghp_FAKE_TOKEN") {
		t.Fatalf("credentials file does not contain the PAT — test premise broken")
	}
	if perm != 0o600 {
		t.Errorf(".git-credentials (contains plaintext GitHub PAT) has permissions %o, want 0600 (owner-only)", perm)
	}
}
