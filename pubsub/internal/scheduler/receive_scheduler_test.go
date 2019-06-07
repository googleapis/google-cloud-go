// Copyright 2019 Google LLC
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

package scheduler_test

import (
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/internal/scheduler"
)

type pair struct {
	k string
	v int
}

func TestReceiveScheduler_Put_Basic(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	keysHandled := map[string]chan int{}
	handle := func(itemi interface{}) {
		item := itemi.(pair)
		keysHandled[item.k] <- item.v
	}

	// If these values are too high, the race detector will fail with "race: limit on 8128 simultaneously alive goroutines is exceeded, dying"
	numItems := 100
	numKeys := 10

	s := scheduler.NewReceiveScheduler(1)
	defer s.Shutdown()
	for ki := 0; ki < numKeys; ki++ {
		k := fmt.Sprintf("some_key_%d", ki)
		keysHandled[k] = make(chan int, numItems)
	}

	for ki := 0; ki < numKeys; ki++ {
		k := fmt.Sprintf("some_key_%d", ki)
		go func() {
			for i := 0; i < numItems; i++ {
				select {
				case <-done:
					return
				default:
				}
				if err := s.Add(k, pair{k: k, v: i}, handle); err != nil {
					t.Error(err)
				}
			}
		}()
	}

	for ki := 0; ki < numKeys; ki++ {
		k := fmt.Sprintf("some_key_%d", ki)
		for want := 0; want < numItems; want++ {
			select {
			case got := <-keysHandled[k]:
				if got != want {
					t.Fatalf("%s: got %d, want %d", k, got, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s: expected key %s - item %d to be handled but never was", k, k, want)
			}
		}
	}
}

// Scheduler schedules many items of one key in order even when there are
// many workers.
func TestReceiveScheduler_Put_ManyWithOneKey(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	recvd := make(chan int)
	handle := func(itemi interface{}) {
		recvd <- itemi.(int)
	}

	// If these values are too high, the race detector will fail with "race: limit on 8128 simultaneously alive goroutines is exceeded, dying"
	numItems := 10000
	s := scheduler.NewReceiveScheduler(10)
	defer s.Shutdown()

	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case <-done:
				return
			default:
			}
			if err := s.Add("some-key", i, handle); err != nil {
				t.Error(err)
			}
		}
	}()

	for want := 0; want < numItems; want++ {
		select {
		case got := <-recvd:
			if got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for item %d to be handled", want)
		}
	}
}

// FlushAndStop flushes all messages. (it does not wait for their completion)
func TestReceiveScheduler_FlushAndStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input map[string][]int
	}{
		{
			name:  "two messages with the same key",
			input: map[string][]int{"foo": {1, 2}},
		},
		{
			name:  "two messages with different keys",
			input: map[string][]int{"foo": {1}, "bar": {2}},
		},
		{
			name:  "two messages with no key",
			input: map[string][]int{"": {1, 2}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recvd := make(chan int, 10)
			handle := func(itemi interface{}) {
				recvd <- itemi.(int)
			}
			s := scheduler.NewReceiveScheduler(1)
			for k, vs := range tc.input {
				for _, v := range vs {
					if err := s.Add(k, v, handle); err != nil {
						t.Fatal(err)
					}
				}
			}

			go func() {
				s.Shutdown()
			}()

			time.Sleep(10 * time.Millisecond)

			select {
			case <-recvd:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for first message to arrive")
			}

			select {
			case <-recvd:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for second message to arrive")
			}
		})
	}
}
