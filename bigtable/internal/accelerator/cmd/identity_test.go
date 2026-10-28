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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2/google"
)

func TestEmailFromCredsJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "service account key",
			in:   `{"type":"service_account","client_email":"sa@proj.iam.gserviceaccount.com","private_key":"-----BEGIN..."}`,
			want: "sa@proj.iam.gserviceaccount.com",
		},
		{
			name: "impersonated",
			in:   `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/target@proj.iam.gserviceaccount.com:generateAccessToken"}`,
			want: "target@proj.iam.gserviceaccount.com",
		},
		{
			name: "workload identity federation",
			in:   `{"type":"external_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/wif@proj.iam.gserviceaccount.com:generateAccessToken"}`,
			want: "wif@proj.iam.gserviceaccount.com",
		},
		{
			name: "authorized user has no local email",
			in:   `{"type":"authorized_user","client_id":"abc.apps.googleusercontent.com","refresh_token":"x"}`,
			want: "",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "garbage",
			in:   "not json",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailFromCredsJSON([]byte(tc.in)); got != tc.want {
				t.Errorf("emailFromCredsJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEmailFromImpersonationURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{
			in:   "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/x@y.iam.gserviceaccount.com:generateAccessToken",
			want: "x@y.iam.gserviceaccount.com",
		},
		{in: "https://example.com/no/marker", want: ""},
		{in: "", want: ""},
	} {
		if got := emailFromImpersonationURL(tc.in); got != tc.want {
			t.Errorf("emailFromImpersonationURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWriteIdentity_WritesDocNextToSocket(t *testing.T) {
	dir := t.TempDir()
	udsPath := filepath.Join(dir, "sock")

	// Point ADC at a service-account key so resolution is local and
	// deterministic (no metadata server, no network).
	keyPath := filepath.Join(dir, "key.json")
	const email = "unit@proj.iam.gserviceaccount.com"
	key := `{"type":"service_account","project_id":"proj","client_email":"` + email + `",` +
		`"private_key_id":"kid","token_uri":"https://oauth2.googleapis.com/token",` +
		`"private_key":"-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"}`
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", keyPath)

	scopes := []string{"https://www.googleapis.com/auth/bigtable.data"}
	// Resolve ADC once (as main.go does) and derive the principal from those same
	// creds, rather than a second FindDefaultCredentials call inside resolution.
	creds, err := google.FindDefaultCredentials(t.Context(), scopes...)
	if err != nil {
		t.Fatalf("FindDefaultCredentials: %v", err)
	}
	principal, err := principalFromCreds(t.Context(), creds)
	if err != nil {
		t.Fatalf("principalFromCreds: %v", err)
	}
	if err := writeIdentity(udsPath, principal, scopes); err != nil {
		t.Fatalf("writeIdentity: %v", err)
	}

	path := filepath.Join(dir, identityFilename)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("identity file not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("identity file mode = %o, want 600", perm)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc identityDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal identity: %v", err)
	}
	if doc.Principal != email {
		t.Errorf("principal = %q, want %q", doc.Principal, email)
	}
	if len(doc.Scopes) != 1 || doc.Scopes[0] != scopes[0] {
		t.Errorf("scopes = %v, want %v", doc.Scopes, scopes)
	}
	// The document must never contain the private key material.
	if strings.Contains(string(b), "PRIVATE KEY") {
		t.Errorf("identity doc leaked key material: %s", b)
	}
}

func TestPrincipalFromCreds_NoGCEFallthroughForJSONOrNilCreds(t *testing.T) {
	const gceEmail = "vm-default-sa@gce.iam.gserviceaccount.com"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Metadata-Flavor", "Google")
		if strings.HasSuffix(r.URL.Path, "/service-accounts/default/email") {
			_, _ = w.Write([]byte(gceEmail))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(ts.URL, "http://"))

	for _, tc := range []struct {
		name  string
		creds *google.Credentials
		want  string
	}{
		{
			name:  "nil credentials do not query GCE metadata",
			creds: nil,
			want:  "",
		},
		{
			name: "authorized_user JSON does not fall through to GCE metadata",
			creds: &google.Credentials{
				JSON: []byte(`{"type":"authorized_user","client_id":"abc","refresh_token":"tok"}`),
			},
			want: "",
		},
		{
			name: "direct WIF external_account JSON without impersonation does not fall through to GCE metadata",
			creds: &google.Credentials{
				JSON: []byte(`{"type":"external_account","audience":"//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/p/providers/pr"}`),
			},
			want: "",
		},
		{
			name:  "GCE metadata credentials without JSON resolve via metadata server",
			creds: &google.Credentials{},
			want:  gceEmail,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := principalFromCreds(t.Context(), tc.creds)
			if err != nil {
				t.Fatalf("principalFromCreds: %v", err)
			}
			if got != tc.want {
				t.Errorf("principalFromCreds() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name            string
		userAgent       string
		callerUserAgent string
		want            string
	}{
		{
			name:            "prefers --user-agent",
			userAgent:       "ua-primary",
			callerUserAgent: "ua-fallback",
			want:            "ua-primary",
		},
		{
			name:            "falls back to --caller-user-agent",
			userAgent:       "",
			callerUserAgent: " cbt-python-data/3.0.0 ",
			want:            "cbt-python-data/3.0.0",
		},
		{
			name:            "both empty",
			userAgent:       "   ",
			callerUserAgent: "",
			want:            "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveUserAgent(tc.userAgent, tc.callerUserAgent); got != tc.want {
				t.Errorf("resolveUserAgent(%q, %q) = %q, want %q", tc.userAgent, tc.callerUserAgent, got, tc.want)
			}
		})
	}
}
