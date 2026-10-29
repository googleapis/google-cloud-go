// Copyright 2026 Google LLC
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2/google"
)

// identityFilename is the name of the identity document written alongside the
// UDS. The spawning client reads it to verify the daemon resolved the same
// principal it did, then deletes it.
const identityFilename = "identity.json"

// gceMetadataCredType identifies credentials resolved from the GCE/GKE
// link-local metadata server, which carry no credential JSON document.
const gceMetadataCredType = "gce_metadata"

// identityDoc is the on-disk handshake payload. It holds only identifiers --
// the resolved principal, credential source type, and effective scopes --
// never access/refresh tokens or private-key bytes.
type identityDoc struct {
	Principal string   `json:"principal"`
	Type      string   `json:"type"`
	Scopes    []string `json:"scopes"`
}

// writeIdentity writes the daemon's resolved principal, credential type, and
// scopes to identity.json next to udsPath before the UDS is bound.
// An empty principal or type is written as-is so the client treats unknown
// identities as unverifiable and falls back to its native path.
func writeIdentity(udsPath, principal, credType string, scopes []string) error {
	doc := identityDoc{Principal: principal, Type: credType, Scopes: scopes}
	b, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal identity doc: %w", err)
	}
	path := filepath.Join(filepath.Dir(udsPath), identityFilename)
	// 0600: identifier only, but keep it same-uid readable and nothing more.
	// The mode is a no-op on Windows, which has no POSIX bits; there the file
	// inherits its directory's ACL, same as the socket beside it. That is
	// acceptable because the document carries no secret -- only a principal
	// email and scope list.
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// identityFromCreds returns the principal email and credential source type of
// pre-resolved ADC using only local JSON or the link-local GCE metadata server.
// It returns empty strings with a nil error when no local source exposes an
// identifier, or a non-nil error if a metadata-server lookup fails.
func identityFromCreds(ctx context.Context, creds *google.Credentials) (principal, credType string, err error) {
	if creds == nil {
		return "", "", nil
	}
	if len(creds.JSON) > 0 {
		principal, credType = identityFromCredsJSON(creds.JSON)
		return principal, credType, nil
	}
	if metadata.OnGCE() {
		// Link-local metadata server; no external egress, no IAM permission.
		email, err := metadata.EmailWithContext(ctx, "default")
		if err != nil {
			return "", "", fmt.Errorf("resolve principal from metadata server: %w", err)
		}
		return strings.TrimSpace(email), gceMetadataCredType, nil
	}
	return "", "", nil
}

// identityFromCredsJSON extracts the principal email and credential type from
// ADC JSON without network calls. Returns empty strings on empty or invalid JSON.
func identityFromCredsJSON(b []byte) (email, credType string) {
	if len(b) == 0 {
		return "", ""
	}
	var doc struct {
		Type                           string `json:"type"`
		ClientEmail                    string `json:"client_email"`
		ServiceAccountImpersonationURL string `json:"service_account_impersonation_url"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", ""
	}
	credType = strings.TrimSpace(doc.Type)
	if email = strings.TrimSpace(doc.ClientEmail); email != "" {
		return email, credType
	}
	return strings.TrimSpace(emailFromImpersonationURL(doc.ServiceAccountImpersonationURL)), credType
}

// emailFromImpersonationURL pulls "<email>" out of a URL shaped like
// ".../serviceAccounts/<email>:generateAccessToken". Returns "" if the URL does
// not match that shape.
func emailFromImpersonationURL(url string) string {
	const marker = "/serviceAccounts/"
	i := strings.Index(url, marker)
	if i < 0 {
		return ""
	}
	rest := url[i+len(marker):]
	if j := strings.IndexByte(rest, ':'); j >= 0 {
		rest = rest[:j]
	}
	if k := strings.IndexByte(rest, '/'); k >= 0 {
		rest = rest[:k]
	}
	return rest
}
