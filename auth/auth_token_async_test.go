// Copyright 2025 Google LLC
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

package auth

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"testing"
	"time"
)

type controllableTokenProvider struct {
	mu    sync.Mutex
	count int
	tok   *Token
	err   error
	block chan struct{}
}

func (p *controllableTokenProvider) Token(ctx context.Context) (*Token, error) {
	if ch := p.getBlockChan(); ch != nil {
		<-ch
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	return p.tok, p.err
}

func (p *controllableTokenProvider) getBlockChan() chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.block
}

func (p *controllableTokenProvider) setBlockChan(ch chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.block = ch
}

func (p *controllableTokenProvider) getCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

func TestCachedTokenProvider_TokenAsyncRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("attempt-%d", i), func(t *testing.T) {
			now := time.Now()
			timeNow = func() time.Time { return now }
			defer func() { timeNow = time.Now }()

			tp := &controllableTokenProvider{}
			ctp := NewCachedTokenProvider(tp, &CachedTokenProviderOptions{
				ExpireEarly: 2 * time.Second,
			}).(*cachedTokenProvider)

			// 1. Cache a stale token.
			tp.tok = &Token{Value: "initial", Expiry: now.Add(1 * time.Second)}
			if _, err := ctp.Token(context.Background()); err != nil {
				t.Fatalf("initial Token() failed: %v", err)
			}
			if got, want := tp.getCount(), 1; got != want {
				t.Fatalf("tp.count = %d; want %d", got, want)
			}
			if got, want := ctp.tokenState(), stale; got != want {
				t.Fatalf("tokenState = %v; want %v", got, want)
			}

			// 2. Setup for refresh.
			tp.setBlockChan(make(chan struct{}))
			tp.tok = &Token{Value: "refreshed", Expiry: now.Add(1 * time.Hour)}

			// 3. Concurrently call Token to trigger async refresh.
			var wg sync.WaitGroup
			numGoroutines := 20 * (i + 1)
			wg.Add(numGoroutines)
			for i := 0; i < numGoroutines; i++ {
				go func() {
					defer wg.Done()
					runtime.Gosched()
					ctp.Token(context.Background())
				}()
			}

			// 4. Unblock refresh and wait for all goroutines to finish.
			time.Sleep(100 * time.Millisecond) // give time for goroutines to run
			close(tp.getBlockChan())
			wg.Wait()
			time.Sleep(100 * time.Millisecond) // give time for async refresh to complete

			// 5. Check results.
			if got, want := tp.getCount(), 2; got != want {
				t.Errorf("tp.count = %d; want %d. This indicates a race condition where multiple refreshes were triggered.", got, want)
			}
			if got, want := ctp.tokenState(), fresh; got != want {
				t.Errorf("tokenState = %v; want %v", got, want)
			}
			tok, err := ctp.Token(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got, want := tok.Value, "refreshed"; got != want {
				t.Errorf("tok.Value = %q; want %q", got, want)
			}
		})
	}
}
func TestCachedTokenProvider_GracefulDegradation(t *testing.T) {
	now := time.Now()
	timeNow = func() time.Time { return now }
	defer func() { timeNow = time.Now }()

	tp := &controllableTokenProvider{}
	ctp := NewCachedTokenProvider(tp, &CachedTokenProviderOptions{
		ExpireEarly: 2 * time.Second,
	}).(*cachedTokenProvider)

	// 1. Cache a stale token.
	staleTok := &Token{Value: "initial", Expiry: now.Add(1 * time.Second)}
	tp.tok = staleTok
	if _, err := ctp.Token(context.Background()); err != nil {
		t.Fatalf("initial Token() failed: %v", err)
	}

	if got, want := ctp.tokenState(), stale; got != want {
		t.Fatalf("tokenState = %v; want %v", got, want)
	}

	// 2. Set provider to return transient error.
	tp.mu.Lock()
	tp.tok = nil
	tp.err = errors.New("transient error")
	tp.mu.Unlock()

	// 3. Call Token(), which should trigger async refresh and immediately return the stale token.
	tok1, err := ctp.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tok1.Value, "initial"; got != want {
		t.Errorf("got token %q, want %q", got, want)
	}

	// 4. Wait for background goroutine to execute and fail.
	start := time.Now()
	for {
		ctp.mu.Lock()
		isErr := ctp.isRefreshErr
		ctp.mu.Unlock()
		if isErr {
			break
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("timeout waiting for background refresh to fail")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 5. Verify the stale token is still preserved and returned.
	tok2, err := ctp.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tok2.Value, "initial"; got != want {
		t.Errorf("got token %q, want %q", got, want)
	}
}


