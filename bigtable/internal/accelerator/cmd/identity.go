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

// identityDoc is the on-disk handshake payload. It holds only identifiers --
// the resolved principal (a service-account email) and the scopes the daemon
// dials with -- never access/refresh tokens or private-key bytes.
type identityDoc struct {
	Principal string   `json:"principal"`
	Scopes    []string `json:"scopes"`
}

// resolveAndWriteIdentity resolves the daemon's Application Default Credentials
// locally (no token introspection) and writes the resulting principal + scopes
// to identity.json in the same directory as udsPath. It must be called before
// the UDS is bound so the file is present by the time the client can connect.
//
// An empty principal (e.g. plain user ADC, which exposes no local email) is
// written as-is; the client treats "no principal" as unverifiable and falls
// back to its native path rather than risking an identity mismatch.
func resolveAndWriteIdentity(ctx context.Context, udsPath string, scopes []string) error {
	principal, err := resolvePrincipal(ctx, scopes)
	if err != nil {
		return err
	}
	doc := identityDoc{Principal: principal, Scopes: scopes}
	b, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal identity doc: %w", err)
	}
	path := filepath.Join(filepath.Dir(udsPath), identityFilename)
	// 0600: identifier only, but keep it same-uid readable and nothing more.
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// resolvePrincipal returns the service-account email of the daemon's ADC using
// only local sources: the credential JSON for keyed creds (SA key, impersonated,
// Workload Identity Federation) and the link-local metadata server for GCE/GKE
// creds. Returns "" (no error) when no principal can be resolved locally.
func resolvePrincipal(ctx context.Context, scopes []string) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, scopes...)
	if err != nil {
		return "", fmt.Errorf("find default credentials: %w", err)
	}
	if email := emailFromCredsJSON(creds.JSON); email != "" {
		return email, nil
	}
	if metadata.OnGCE() {
		// Link-local metadata server; no external egress, no IAM permission.
		if email, err := metadata.EmailWithContext(ctx, "default"); err == nil {
			return email, nil
		}
	}
	return "", nil
}

// emailFromCredsJSON extracts the principal email from ADC credential JSON
// without any network call. It handles service-account keys (client_email) and
// impersonated / external-account creds (the target email embedded in the
// impersonation URL). Returns "" for user creds or unrecognized shapes.
func emailFromCredsJSON(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var doc struct {
		Type                           string `json:"type"`
		ClientEmail                    string `json:"client_email"`
		ServiceAccountImpersonationURL string `json:"service_account_impersonation_url"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return ""
	}
	if doc.ClientEmail != "" {
		return doc.ClientEmail
	}
	return emailFromImpersonationURL(doc.ServiceAccountImpersonationURL)
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
