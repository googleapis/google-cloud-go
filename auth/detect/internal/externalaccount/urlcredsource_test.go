// Copyright 2023 Google LLC
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

package externalaccount

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/auth/internal/internaldetect"
)

func TestRetrieveURLSubjectToken_Text(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := http.MethodGet; r.Method != want {
			t.Errorf("got %v, want %v", r.Method, want)
		}
		if r.Header.Get("Metadata") != "True" {
			t.Errorf("Metadata header not properly included.")
		}
		w.Write([]byte("testTokenValue"))
	}))
	defer ts.Close()
	heads := make(map[string]string)
	heads["Metadata"] = "True"
	cs := internaldetect.CredentialSource{
		URL:     ts.URL,
		Format:  internaldetect.Format{Type: fileTypeText},
		Headers: heads,
	}
	tfc := testFileConfig
	tfc.CredentialSource = cs

	base, err := tfc.parse()
	if err != nil {
		t.Fatalf("parse() failed %v", err)
	}

	got, err := base.subjectToken(context.Background())
	if err != nil {
		t.Fatalf("retrieveSubjectToken() failed: %v", err)
	}
	if want := "testTokenValue"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Checking that retrieveSubjectToken properly defaults to type text
func TestRetrieveURLSubjectToken_Untyped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := http.MethodGet; r.Method != want {
			t.Errorf("got %v, want %v", r.Method, want)
		}
		w.Write([]byte("testTokenValue"))
	}))
	defer ts.Close()
	cs := internaldetect.CredentialSource{
		URL: ts.URL,
	}
	tfc := testFileConfig
	tfc.CredentialSource = cs

	base, err := tfc.parse()
	if err != nil {
		t.Fatalf("parse() failed %v", err)
	}

	got, err := base.subjectToken(context.Background())
	if err != nil {
		t.Fatalf("Failed to retrieve URL subject token: %v", err)
	}
	if want := "testTokenValue"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRetrieveURLSubjectToken_JSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, "GET"; got != want {
			t.Errorf("got %v, want %v", r.Method, want)
		}
		w.Write([]byte(`{"SubjToken":"testTokenValue"}`))
	}))
	defer ts.Close()
	cs := internaldetect.CredentialSource{
		URL:    ts.URL,
		Format: internaldetect.Format{Type: fileTypeJSON, SubjectTokenFieldName: "SubjToken"},
	}
	tfc := testFileConfig
	tfc.CredentialSource = cs

	base, err := tfc.parse()
	if err != nil {
		t.Fatalf("tfc.parse() = %v", err)
	}

	got, err := base.subjectToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "testTokenValue"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
