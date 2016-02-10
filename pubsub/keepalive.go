// Copyright 2016 Google Inc. All Rights Reserved.
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

package pubsub

import (
	"sync"
	"time"

	"golang.org/x/net/context"
)

// keepAlive keeps track of which Messages need to have their deadline extended, and
// periodically extends them.
// Messages are tracked by Ack ID.
type keepAlive struct {
	Client        Client
	Ctx           context.Context // The context to use when extending deadlines.
	Sub           string          // The full name of the subscription.
	ExtensionTick chan time.Time  // ExtenstionTick supplies the frequency with which to make extension requests.
	Deadline      time.Duration   // How long to extend messages for. Should be greater than ExtensionTick frequency.

	ackIDs map[string]struct{}
	done   chan struct{}
	wg     sync.WaitGroup
}

// Start initiates the deadline extension loop.  Stop must be called once keepAlive is no longer needed.
// add and remove may be used to add and remove messages to be kept alive.
func (ka *keepAlive) Start(add, remove <-chan string) {
	ka.ackIDs = make(map[string]struct{})
	ka.done = make(chan struct{})
	ka.wg.Add(1)
	go func() {
		defer ka.wg.Done()
		done := false
		for {
			select {
			case ackID := <-add:
				ka.ackIDs[ackID] = struct{}{}
			case ackID := <-remove:
				delete(ka.ackIDs, ackID)
			case <-ka.done:
				done = true
			case <-ka.ExtensionTick:
				ackIDs := ka.getAckIds()
				ka.wg.Add(1)
				go func() {
					defer ka.wg.Done()
					ka.extendDeadlines(ackIDs)
				}()
			}
			if done && len(ka.ackIDs) == 0 {
				return
			}
		}
	}()
}

// Stop waits until all added ackIDs have been removed, and cleans up resources.
func (ka *keepAlive) Stop() {
	ka.done <- struct{}{}
	ka.wg.Wait()
}

func (ka *keepAlive) getAckIds() []string {
	ids := []string{}
	for id, _ := range ka.ackIDs {
		ids = append(ids, id)
	}
	return ids
}

func (ka *keepAlive) extendDeadlines(ackIDs []string) {
	// TODO: split into separate requests if there are too many ackIDs.
	if len(ackIDs) > 0 {
		_ = ka.Client.s.modifyAckDeadline(ka.Ctx, ka.Sub, ka.Deadline, ackIDs)
	}
	// TODO: retry on error.  NOTE: if we ultimately fail to extend deadlines here, the messages will be redelivered, which is OK.
	// TODO: ensure that messages are not kept alive forever, via a max extension duration. That can be managed outside of keepAlive.
}
