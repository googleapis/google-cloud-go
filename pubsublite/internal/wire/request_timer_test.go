// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package wire

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/pubsublite/internal/test"
)

func TestMinDuration(t *testing.T) {
	for _, tc := range []struct {
		a    time.Duration
		b    time.Duration
		want time.Duration
	}{
		{
			a:    10 * time.Millisecond,
			b:    10 * time.Millisecond,
			want: 10 * time.Millisecond,
		},
		{
			a:    10 * time.Millisecond,
			b:    9 * time.Millisecond,
			want: 9 * time.Millisecond,
		},
		{
			a:    5 * time.Millisecond,
			b:    5 * time.Second,
			want: 5 * time.Millisecond,
		},
	} {
		t.Run(fmt.Sprintf("%s %s", tc.a, tc.b), func(t *testing.T) {
			if got := minDuration(tc.a, tc.b); got != tc.want {
				t.Errorf("minDuration(%v, %v): got %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestRequestTimerStop(t *testing.T) {
	const timeout = 5 * time.Millisecond
	onTimeout := func() {
		t.Error("onTimeout should not be called")
	}

	rt := newRequestTimer(timeout, onTimeout, errors.New("unused"))
	rt.Stop()
	time.Sleep(2 * timeout)

	if err := rt.ResolveError(nil); err != nil {
		t.Errorf("ResolveError() got gotErr: %v", err)
	}
	wantErr := errors.New("original error")
	if gotErr := rt.ResolveError(wantErr); !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("ResolveError() got err: %v, want err: %v", gotErr, wantErr)
	}
}

func TestRequestTimerExpires(t *testing.T) {
	const timeout = 5 * time.Millisecond
	timeoutErr := errors.New("on timeout")

	expired := test.NewCondition("request timer expired")
	onTimeout := func() {
		expired.SetDone()
	}

	rt := newRequestTimer(timeout, onTimeout, timeoutErr)
	expired.WaitUntilDone(t, serviceTestWaitTimeout)

	if gotErr := rt.ResolveError(nil); !test.ErrorEqual(gotErr, timeoutErr) {
		t.Errorf("ResolveError() got err: %v, want err: %v", gotErr, timeoutErr)
	}
	if gotErr := rt.ResolveError(errors.New("ignored")); !test.ErrorEqual(gotErr, timeoutErr) {
		t.Errorf("ResolveError() got err: %v, want err: %v", gotErr, timeoutErr)
	}
}

func TestStreamIdleTimerExpires(t *testing.T) {
	const timeout = 5 * time.Millisecond
	expired := test.NewCondition("timer expired")

	st := newStreamIdleTimer(timeout, expired.SetDone)
	defer st.Shutdown()
	st.Restart()
	expired.WaitUntilDone(t, serviceTestWaitTimeout)
}

func TestStreamIdleTimerRestart(t *testing.T) {
	const timeout = 20 * time.Millisecond
	const delta = 15 * time.Millisecond
	expired := test.NewCondition("timer expired")

	st := newStreamIdleTimer(timeout, expired.SetDone)
	defer st.Shutdown()
	st.Restart()
	time.Sleep(delta)
	expired.VerifyNotDone(t)
	st.Restart()
	time.Sleep(delta)
	expired.VerifyNotDone(t)
	expired.WaitUntilDone(t, serviceTestWaitTimeout)
}

func TestStreamIdleTimerStop(t *testing.T) {
	const timeout = 5 * time.Millisecond
	onTimeout := func() {
		t.Error("onTimeout should not be called")
	}

	st := newStreamIdleTimer(timeout, onTimeout)
	defer st.Shutdown()
	st.Restart()
	st.Stop()
	time.Sleep(2 * timeout)
}

func TestStreamIdleTimerShutdown(t *testing.T) {
	const timeout = 5 * time.Millisecond
	onTimeout := func() {
		t.Error("onTimeout should not be called")
	}

	st := newStreamIdleTimer(timeout, onTimeout)
	st.Restart()
	st.Shutdown()
	time.Sleep(2 * timeout)
}
