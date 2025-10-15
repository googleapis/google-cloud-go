// Copyright 2016 Google LLC
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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOnGCE_Stress(t *testing.T) {
	ctx := context.Background()
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	var last bool
	for i := 0; i < 100; i++ {
		onGCEOnce = sync.Once{}

		now := OnGCEWithContext(ctx)
		if i > 0 && now != last {
			t.Errorf("%d. changed from %v to %v", i, last, now)
		}
		last = now
	}
	t.Logf("OnGCE() = %v", last)
}

func TestOnGCE_Force(t *testing.T) {
	ctx := context.Background()
	onGCEOnce = sync.Once{}
	old := os.Getenv(metadataHostEnv)
	defer os.Setenv(metadataHostEnv, old)
	os.Setenv(metadataHostEnv, "127.0.0.1")
	if !OnGCEWithContext(ctx) {
		t.Error("OnGCE() = false; want true")
	}
}

func TestOnGCE_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	onGCEOnce = sync.Once{}
	if OnGCEWithContext(ctx) {
		t.Error("OnGCE() = true; want false")
	}
}

func TestOnGCE_CancelTryHarder(t *testing.T) {
	// If system info suggests GCE, we allow extra time for the
	// probe with higher latency (HTTP or DNS) to return. In this
	// test, the system info suggest GCE, the DNS probe fails
	// immediately, and the HTTP probe would succeed after 750ms.
	// However, the user-provided context deadline is 500ms. GCE
	// detection should fail, respecting the provided context.
	//
	// NOTE: This code could create a data race if tests are run
	// in parallel.
	origSystemInfoSuggestsGCE := systemInfoSuggestsGCE
	origMetadataRequestStrategy := metadataRequestStrategy
	origDNSRequestStrategy := dnsRequestStrategy
	systemInfoSuggestsGCE = func() bool { return true }
	metadataRequestStrategy = func(_ context.Context, _ *http.Client, resc chan bool) {
		time.Sleep(750 * time.Millisecond)
		resc <- true
	}
	dnsRequestStrategy = func(_ context.Context, resc chan bool) {
		resc <- false
	}
	defer func() {
		systemInfoSuggestsGCE = origSystemInfoSuggestsGCE
		metadataRequestStrategy = origMetadataRequestStrategy
		dnsRequestStrategy = origDNSRequestStrategy
	}()

	// Set deadline upper-limit to 500ms
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Set HTTP deadline to 1s
	c := NewClient(&http.Client{Transport: sleepyTransport{1 * time.Second}})

	start := time.Now()
	if c.OnGCEWithContext(ctx) {
		t.Error("OnGCE() = true; want false")
	}

	// Should have returned around 500ms, but account for some scheduling budget
	if time.Since(start) > 510*time.Millisecond {
		t.Error("OnGCE() did not return within deadline")
	}
}

func TestOverrideUserAgent(t *testing.T) {
	ctx := context.Background()
	const userAgent = "my-user-agent"
	rt := &rrt{}
	c := NewClient(&http.Client{Transport: userAgentTransport{userAgent, rt}})
	c.GetWithContext(ctx, "foo")
	if got, want := rt.gotUserAgent, userAgent; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetFailsOnBadURL(t *testing.T) {
	ctx := context.Background()
	c := NewClient(http.DefaultClient)
	t.Setenv(metadataHostEnv, "host:-1")
	_, err := c.GetWithContext(ctx, "suffix")
	if err == nil {
		t.Errorf("got %v, want non-nil error", err)
	}
}

func TestGet_LeadingSlash(t *testing.T) {
	want := "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/identity?audience=http://example.com"
	tests := []struct {
		name   string
		suffix string
	}{
		{
			name:   "without leading slash",
			suffix: "instance/service-accounts/default/identity?audience=http://example.com",
		},
		{
			name:   "with leading slash",
			suffix: "/instance/service-accounts/default/identity?audience=http://example.com",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ct := &captureTransport{}
			c := NewClient(&http.Client{Transport: ct})
			_, _ = c.GetWithContext(ctx, tc.suffix)
			if ct.url != want {
				t.Fatalf("got %v, want %v", ct.url, want)
			}
		})
	}
}

type captureTransport struct {
	url string
}

func (ct *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ct.url = req.URL.String()
	return &http.Response{Body: io.NopCloser(io.Reader(bytes.NewReader(nil)))}, nil
}

type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

type rrt struct {
	gotUserAgent string
}

func (r *rrt) RoundTrip(req *http.Request) (*http.Response, error) {
	r.gotUserAgent = req.Header.Get("User-Agent")
	return &http.Response{Body: io.NopCloser(bytes.NewReader(nil))}, nil
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name        string
		timesToFail int
		failCode    int
		failErr     error
		response    string
		expectError bool
	}{
		{
			name:     "no retries",
			response: "test",
		},
		{
			name:        "retry 500 once",
			response:    "test",
			failCode:    500,
			timesToFail: 1,
		},
		{
			name:        "retry io.ErrUnexpectedEOF once",
			response:    "test",
			failErr:     io.ErrUnexpectedEOF,
			timesToFail: 1,
		},
		{
			name:        "retry io.ErrUnexpectedEOF permanent",
			failErr:     io.ErrUnexpectedEOF,
			timesToFail: maxRetryAttempts + 1,
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ft := &failingTransport{
				timesToFail: tt.timesToFail,
				failCode:    tt.failCode,
				failErr:     tt.failErr,
				response:    tt.response,
			}
			c := NewClient(&http.Client{Transport: ft})
			s, err := c.GetWithContext(ctx, "")
			if tt.expectError && err == nil {
				t.Fatalf("did not receive expected error")
			} else if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedCount := ft.failedAttempts + 1
			if tt.expectError {
				expectedCount = ft.failedAttempts
			} else if s != tt.response {
				// Responses are only meaningful if err == nil
				t.Fatalf("c.Get() = %q, want %q", s, tt.response)
			}

			if ft.called != expectedCount {
				t.Fatalf("failed %d times, want %d", ft.called, expectedCount)
			}
		})
	}
}

func TestClientGetWithContext(t *testing.T) {
	tests := []struct {
		name       string
		ctxTimeout time.Duration
		wantErr    bool
	}{
		{
			name:       "ok",
			ctxTimeout: 1 * time.Second,
		},
		{
			name:       "times out",
			ctxTimeout: 200 * time.Millisecond,
			wantErr:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.ctxTimeout)
			defer cancel()
			c := NewClient(&http.Client{Transport: sleepyTransport{500 * time.Millisecond}})
			_, err := c.GetWithContext(ctx, "foo")
			if tc.wantErr && err == nil {
				t.Fatal("c.GetWithContext() == nil, want an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("c.GetWithContext() = %v, want nil", err)
			}
		})
	}
}

type sleepyTransport struct {
	delay time.Duration
}

func (s sleepyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Context().Done()
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(s.delay):
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("I woke up"))}, nil
}

type failingTransport struct {
	timesToFail int
	failCode    int
	failErr     error
	response    string

	failedAttempts int
	called         int
}

func (r *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.called++
	if r.failedAttempts < r.timesToFail {
		r.failedAttempts++
		if r.failErr != nil {
			return nil, r.failErr
		}
		return &http.Response{StatusCode: r.failCode}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(r.response))}, nil
}

func TestNewClient(t *testing.T) {
	customClient := &http.Client{}

	tests := []struct {
		name      string
		in        *http.Client
		wantHC    *http.Client
		isDefault bool
	}{
		{
			name:      "nil returns default",
			in:        nil,
			isDefault: true,
		},
		{
			name:   "custom client",
			in:     customClient,
			wantHC: customClient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewClient(tt.in)
			if tt.isDefault {
				if got != defaultClient {
					t.Errorf("NewClient() = %p, want %p", got, defaultClient)
				}
			}
			if tt.wantHC != nil {
				if got.hc != tt.wantHC {
					t.Errorf("NewClient().hc = %p, want %p", got.hc, tt.wantHC)
				}
				if got.subClient != tt.wantHC {
					t.Errorf("NewClient().hc = %p, want %p", got.hc, tt.wantHC)
				}
			}
		})
	}
}

func TestNewWithOptions(t *testing.T) {
	customClient := &http.Client{}
	customLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name                string
		opts                *Options
		wantHC              *http.Client
		isDefault           bool
		wantSharedTransport bool
		wantSharedHC        bool
		hasCustomLogger     bool
	}{
		{
			name:      "nil opts returns default",
			opts:      nil,
			isDefault: true,
		},
		{
			name: "empty opts returns new isolated client",
			opts: &Options{},
		},
		{
			name:   "with custom client",
			opts:   &Options{Client: customClient},
			wantHC: customClient,
		},
		{
			name:                "UseDefaultClient returns client with default hc",
			opts:                &Options{UseDefaultClient: true},
			wantSharedHC:        true,
			wantSharedTransport: true,
		},
		{
			name:                "UseDefaultClient with custom logger",
			opts:                &Options{UseDefaultClient: true, Logger: customLogger},
			wantSharedHC:        true,
			wantSharedTransport: true,
			hasCustomLogger:     true,
		},
		{
			name:                "UseDefaultClient ignores custom client",
			opts:                &Options{UseDefaultClient: true, Client: customClient},
			wantSharedHC:        true,
			wantSharedTransport: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewWithOptions(tt.opts)

			if tt.isDefault {
				if got != defaultClient {
					t.Errorf("NewWithOptions() = %p, want defaultClient %p", got, defaultClient)
				}
				return // Don't run other checks for this case
			}
			if got == defaultClient {
				t.Errorf("NewWithOptions() = %p, should not be defaultClient", got)
			}
			if tt.wantHC != nil {
				if got.hc != tt.wantHC {
					t.Errorf("NewWithOptions().hc = %p, want %p", got.hc, tt.wantHC)
				}
				if got.subClient != tt.wantHC {
					t.Errorf("NewWithOptions().hc = %p, want %p", got.hc, tt.wantHC)
				}
			}
			if tt.hasCustomLogger {
				if got.logger != customLogger {
					t.Errorf("NewWithOptions().logger = %p, want %p", got.logger, customLogger)
				}
			}

			// Check transport sharing behavior
			if tt.opts != nil && tt.opts.Client == nil {
				got2 := NewWithOptions(tt.opts)
				transportsAreShared := got.hc.Transport == got2.hc.Transport
				if transportsAreShared != tt.wantSharedTransport {
					t.Errorf("Transport sharing mismatch: got %v, want %v", transportsAreShared, tt.wantSharedTransport)
				}
			}

			// Check http client sharing behavior
			if tt.opts != nil && tt.opts.Client == nil {
				got2 := NewWithOptions(tt.opts)
				clientsAreShared := got.hc == got2.hc
				if clientsAreShared != tt.wantSharedHC {
					t.Errorf("Client sharing mismatch: got %v, want %v", clientsAreShared, tt.wantSharedHC)
				}
			}
		})
	}
}

func TestNewWithOptions_UseDefaultClient(t *testing.T) {
	client := NewWithOptions(&Options{UseDefaultClient: true})
	if client.hc != defaultClient.hc {
		t.Errorf("NewWithOptions(UseDefaultClient: true).hc = %p, want %p", client.hc, defaultClient.hc)
	}
	if client.subClient != defaultClient.subClient {
		t.Errorf("NewWithOptions(UseDefaultClient: true).subClient = %p, want %p", client.subClient, defaultClient.subClient)
	}
}

func TestSubscribeUsesSubscribeClient(t *testing.T) {
	subscribeClientUsed := make(chan bool, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Header") == "subscribe" {
			subscribeClientUsed <- true
		} else {
			subscribeClientUsed <- false
		}
		w.Header().Set("Etag", "etag-1")
		fmt.Fprint(w, "initial-value")
	}))
	defer ts.Close()

	t.Setenv(metadataHostEnv, strings.TrimPrefix(ts.URL, "http://"))

	subTransport := &headerTransport{
		header: "X-Test-Header",
		value:  "subscribe",
		base:   http.DefaultTransport,
	}
	subClient := &http.Client{Transport: subTransport}
	hc := newDefaultHTTPClient(true)

	c := &Client{
		hc:        hc,
		subClient: subClient,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c.SubscribeWithContext(ctx, "some/path", func(ctx context.Context, v string, ok bool) error {
		cancel()
		return nil
	})

	select {
	case used := <-subscribeClientUsed:
		if !used {
			t.Error("Subscribe did not use the subscribe client")
		}
	case <-time.After(5 * time.Second):
		// This can happen if the request from the client is never sent.
		t.Fatal("Test timed out")
	}
}

type headerTransport struct {
	header string
	value  string
	base   http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(t.header, t.value)
	return t.base.RoundTrip(req)
}
