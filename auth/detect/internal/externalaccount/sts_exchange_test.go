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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/internal"
)

var (
	clientAuth = clientAuthentication{
		AuthStyle:    auth.StyleInHeader,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	tokenRequest = stsTokenExchangeRequest{
		ActingParty: struct {
			ActorToken     string
			ActorTokenType string
		}{},
		GrantType:          stsGrantType,
		Resource:           "",
		Audience:           "32555940559.apps.googleusercontent.com",
		Scope:              []string{"https://www.googleapis.com/auth/devstorage.full_control"},
		RequestedTokenType: stsTokenType,
		SubjectToken:       "Sample.Subject.Token",
		SubjectTokenType:   "urn:ietf:params:oauth:token-type:jwt",
	}

	responseBody = `{"access_token":"Sample.Access.Token","issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/cloud-platform"}`
)

func TestExchangeToken(t *testing.T) {
	requestbody := "audience=32555940559.apps.googleusercontent.com&grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange&requested_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token&scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fdevstorage.full_control&subject_token=Sample.Subject.Token&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt"
	wantToken := stsTokenExchangeResponse{
		AccessToken:     "Sample.Access.Token",
		IssuedTokenType: stsTokenType,
		TokenType:       internal.TokenTypeBearer,
		ExpiresIn:       3600,
		Scope:           "https://www.googleapis.com/auth/cloud-platform",
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("got %v, want %v", r.Method, http.MethodPost)
		}
		if got, want := r.Header.Get("Authorization"), "Basic cmJyZ25vZ25yaG9uZ28zYmk0Z2I5Z2hnOWc6bm90c29zZWNyZXQ="; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if got, want := string(body), requestbody; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseBody))
	}))
	defer ts.Close()

	headers := http.Header{}
	headers.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := exchangeToken(context.Background(), internal.CloneDefaultClient(), ts.URL, &tokenRequest, clientAuth, headers, nil)
	if err != nil {
		t.Fatalf("exchangeToken() = %v", err)
	}

	if *resp != wantToken {
		t.Errorf("got %v, want %v", *resp, wantToken)
	}
}

func TestExchangeToken_Err(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("what's wrong with this response?"))
	}))
	defer ts.Close()

	headers := http.Header{}
	headers.Add("Content-Type", "application/x-www-form-urlencoded")
	if _, err := exchangeToken(context.Background(), internal.CloneDefaultClient(), ts.URL, &tokenRequest, clientAuth, headers, nil); err == nil {
		t.Errorf("got nil, want an error")
	}
}

func TestExchangeToken_Opts(t *testing.T) {
	optsValues := [][]string{{"foo", "bar"}, {"cat", "pan"}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() = %v", err)
		}
		data, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("url.ParseQuery() = %v", err)
		}
		strOpts, ok := data["options"]
		if !ok {
			t.Errorf(`server didn't receive an "options" field`)
		} else if len(strOpts) < 1 {
			t.Errorf(`"options" field has length 0`)
		}
		var opts map[string]interface{}
		err = json.Unmarshal([]byte(strOpts[0]), &opts)
		if err != nil {
			t.Fatalf(`couldn't parse received "options" field`)
		}
		if len(opts) < 2 {
			t.Errorf("too few options received")
		}

		val, ok := opts["one"]
		if !ok {
			t.Errorf("couldn't find first option parameter")
		} else {
			tOpts1, ok := val.(map[string]interface{})
			if !ok {
				t.Errorf("failed to assert the first option parameter as type testOpts")
			} else {
				if got, want := tOpts1["first"].(string), optsValues[0][0]; got != want {
					t.Errorf("got %v, want %v", got, want)
				}
				if got, want := tOpts1["second"].(string), optsValues[0][1]; got != want {
					t.Errorf("got %v, want %v", got, want)
				}
			}
		}

		val2, ok := opts["two"]
		if !ok {
			t.Errorf("couldn't find second option parameter")
		} else {
			tOpts2, ok := val2.(map[string]interface{})
			if !ok {
				t.Errorf("Failed to assert the second option parameter as type testOpts.")
			} else {
				if got, want := tOpts2["first"].(string), optsValues[1][0]; got != want {
					t.Errorf("got %v, want %v", got, want)
				}
				if got, want := tOpts2["second"].(string), optsValues[1][1]; got != want {
					t.Errorf("got %v, want %v", got, want)
				}
			}
		}
		// Send a proper reply so that no other errors crop up.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseBody))

	}))
	defer ts.Close()
	headers := http.Header{}
	headers.Add("Content-Type", "application/x-www-form-urlencoded")

	type testOpts struct {
		First  string `json:"first"`
		Second string `json:"second"`
	}
	firstOption := testOpts{optsValues[0][0], optsValues[0][1]}
	secondOption := testOpts{optsValues[1][0], optsValues[1][1]}
	inputOpts := make(map[string]interface{})
	inputOpts["one"] = firstOption
	inputOpts["two"] = secondOption
	exchangeToken(context.Background(), internal.CloneDefaultClient(), ts.URL, &tokenRequest, clientAuth, headers, inputOpts)
}
