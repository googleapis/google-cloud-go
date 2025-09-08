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

package pubsub

import "time"

// ShutdownOptions configures the shutdown behavior of the subscriber.
// If not specified, the behavior will default to indefinite processing,
// that is graceful shutdown with no timeout.
type ShutdownOptions struct {
	// Timeout specifies the time the subscriber should wait
	// to shutdown before killing the process.
	// In nack mode, this specifies how long to wait
	// for messages to be nacked after shutdown is initiated.
	//
	// Set to zero to immediately shutdown.
	// Set to a negative value to disable timeout.
	Timeout time.Duration

	// Behavior defines the strategy the subscriber should use when
	// shutting down (wait or nack messages).
	Behavior ShutdownBehavior
}

// ShutdownBehavior defines the strategy the subscriber should take when
// shutting down. Current options are graceful shutdown vs nacking messages.
type ShutdownBehavior int

const (
	// ShutdownBehaviorWaitForProcessing means the subscriber will wait for
	// outstanding messages to be processed before nacking messages finally.
	ShutdownBehaviorWaitForProcessing = iota

	// ShutdownBehaviorNackImmediately means the subscriber will nack all outstanding
	// messages before closing.
	ShutdownBehaviorNackImmediately
)
