// Copyright 2018 Google Inc. All Rights Reserved.
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

package main_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

func TestIntegration_HTTPR(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	if testutil.ProjID() == "" {
		t.Fatal("set GCLOUD_TESTS_GOLANG_PROJECT_ID and GCLOUD_TESTS_GOLANG_KEY")
	}
	// Get a unique temporary filename.
	f, err := ioutil.TempFile("", "httpreplay")
	if err != nil {
		t.Fatal(err)
	}
	replayFilename := f.Name()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(replayFilename)

	if err := exec.Command("go", "build").Run(); err != nil {
		t.Fatalf("running 'go build': %v", err)
	}
	defer os.Remove("./httpr")
	want := run(t, "record", replayFilename)
	got := run(t, "replay", replayFilename)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// mode must be either "record" or "replay".
func run(t *testing.T, mode, filename string) string {
	pport, err := pickPort()
	if err != nil {
		t.Fatal(err)
	}
	cport, err := pickPort()
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := start("-port", pport, "-control-port", cport, "-"+mode, filename, "-debug-headers")
	if err != nil {
		t.Fatal(err)
	}
	defer stop(t, cmd)

	// Wait for the server to come up.
	serverUp := false
	for i := 0; i < 10; i++ {
		if conn, err := net.Dial("tcp", "localhost:"+cport); err == nil {
			conn.Close()
			serverUp = true
			break
		}
		time.Sleep(time.Second)
	}
	if !serverUp {
		t.Fatal("httpr never came up")
	}

	ctx := context.Background()
	tr, err := proxyTransport(pport, cport)
	if err != nil {
		t.Fatal(err)
	}
	var hc *http.Client
	if mode == "record" {
		ts := testutil.TokenSource(ctx, storage.ScopeFullControl)
		hc = &http.Client{
			Transport: &oauth2.Transport{
				Base:   tr,
				Source: ts,
			},
		}
	} else {
		hc = &http.Client{Transport: tr}
	}
	client, err := storage.NewClient(ctx, option.WithHTTPClient(hc))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	b := client.Bucket(testutil.ProjID())
	attrs, err := b.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("name:%s reqpays:%v location:%s sclass:%s",
		attrs.Name, attrs.RequesterPays, attrs.Location, attrs.StorageClass)
}

func start(args ...string) (*exec.Cmd, error) {
	cmd := exec.Command("./httpr", args...)
	if testing.Verbose() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func stop(t *testing.T, cmd *exec.Cmd) {
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
}

// pickPort picks an unused port.
func pickPort() (string, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	l.Close()
	return port, nil
}

func proxyTransport(pport, cport string) (*http.Transport, error) {
	caCert, err := getBody(fmt.Sprintf("http://localhost:%s/authority.cer", cport))
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
		return nil, errors.New("bad CA Cert")
	}
	return &http.Transport{
		Proxy:           http.ProxyURL(&url.URL{Host: "localhost:" + pport}),
		TLSClientConfig: &tls.Config{RootCAs: caCertPool},
	}, nil
}

func getBody(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}
