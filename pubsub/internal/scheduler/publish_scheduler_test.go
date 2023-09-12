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

func TestPublishScheduler_Put_Basic(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	keysHandled := map[string]chan int{}
	handle := func(itemi interface{}) {
		items := itemi.([]pair)
		for _, item := range items {
			keysHandled[item.k] <- item.v
		}
	}
	s := scheduler.NewPublishScheduler(2, handle)
	defer s.FlushAndStop()

	// If these values are too high, the race detector will fail with
	// "race: limit on 8128 simultaneously alive goroutines is exceeded, dying".
	numItems := 100
	numKeys := 10

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
				if err := s.Add(k, pair{k, i}, 1); err != nil {
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
func TestPublishScheduler_Put_ManyWithOneKey(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	recvd := make(chan int)
	handle := func(itemi interface{}) {
		items := itemi.([]int)
		for _, item := range items {
			recvd <- item
		}
	}
	s := scheduler.NewPublishScheduler(10, handle)
	defer s.FlushAndStop()

	// If these values are too high, the race detector will fail with
	// "race: limit on 8128 simultaneously alive goroutines is exceeded, dying".
	numItems := 1000

	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case <-done:
				return
			default:
			}
			if err := s.Add("some-key", i, 1); err != nil {
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

func TestPublishScheduler_DoesntRaceWithPublisher(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	keysHandled := map[string]chan int{}
	handle := func(itemi interface{}) {
		items := itemi.([]pair)
		for _, item := range items {
			keysHandled[item.k] <- item.v
		}
	}
	s := scheduler.NewPublishScheduler(2, handle)
	defer s.FlushAndStop()

	// If these values are too high, the race detector will fail with
	// "race: limit on 8128 simultaneously alive goroutines is exceeded, dying".
	numItems := 100
	numKeys := 10

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
				if err := s.Add(k, pair{k, i}, 1); err != nil {
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

// FlushAndStop blocks until all messages are processed.
func TestPublishScheduler_FlushAndStop(t *testing.T) {
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
			recvd := make(chan int)
			handle := func(itemi interface{}) {
				for _, v := range itemi.([]int) {
					recvd <- v
				}
			}
			s := scheduler.NewPublishScheduler(1, handle)
			for k, vs := range tc.input {
				for _, v := range vs {
					if err := s.Add(k, v, 1); err != nil {
						t.Fatal(err)
					}
				}
			}

			doneFlushing := make(chan struct{})
			go func() {
				s.FlushAndStop()
				close(doneFlushing)
			}()

			time.Sleep(10 * time.Millisecond)

			select {
			case <-doneFlushing:
				t.Fatal("expected FlushAndStop to block until all messages handled, but it didn't")
			default:
			}

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

			select {
			case <-doneFlushing:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for FlushAndStop to finish blocking")
			}
		})
	}
}
