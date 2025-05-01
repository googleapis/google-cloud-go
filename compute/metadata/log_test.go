// Copyright 2024 Google LLC
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

package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var updateGolden = flag.Bool("update-golden", false, "update golden files")

// To update conformance tests in this package run `go test -update_golden`
func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestLog_httpRequest(t *testing.T) {
	golden := "httpRequest.log"
	logger, f := setupLogger(t, golden)
	ctx := context.Background()
	request, err := http.NewRequest(http.MethodPost, "https://example.com/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("foo", "bar")
	logger.DebugContext(ctx, "msg", "request", httpRequest(request, nil))
	f.Close()
	diffTest(t, f.Name(), golden)
}

func TestLog_httpResponse(t *testing.T) {
	golden := "httpResponse.log"
	logger, f := setupLogger(t, golden)
	ctx := context.Background()
	body := []byte(`{"access_token":"token","expires_in":600,"token_type":"Bearer"}`)
	response := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Foo": []string{"bar"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	logger.DebugContext(ctx, "msg", "response", httpResponse(response, body))
	f.Close()
	diffTest(t, f.Name(), golden)
}

func setupLogger(t *testing.T, golden string) (*slog.Logger, *os.File) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), golden)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return logger, f
}

// diffTest is a test helper, testing got against contents of a goldenFile.
func diffTest(t *testing.T, tempFile, goldenFile string) {
	rawGot, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Helper()
	if *updateGolden {
		got := removeLogVariance(t, rawGot)
		if err := os.WriteFile(filepath.Join("testdata", goldenFile), got, os.ModePerm); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(filepath.Join("testdata", goldenFile))
	if err != nil {
		t.Fatal(err)
	}
	got := removeLogVariance(t, rawGot)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch(-want, +got): %s", diff)
	}
}

// removeLogVariance removes parts of log lines that may differ between runs
// and/or machines.
func removeLogVariance(t *testing.T, in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	bs := bytes.Split(in, []byte("\n"))
	for i, b := range bs {
		if len(b) == 0 {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		delete(m, "time")
		if sl, ok := m["sourceLocation"].(map[string]any); ok {
			delete(sl, "file")
			// So that if test cases move around in this file they don't cause
			// failures
			delete(sl, "line")
		}
		b2, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s", b2)
		bs[i] = b2
	}
	return bytes.Join(bs, []byte("\n"))
}
