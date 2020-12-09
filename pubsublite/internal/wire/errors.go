// Copyright 2020 Google LLC
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
)

// Errors exported from this package.
var (
	// ErrOverflow indicates that the publish buffers have overflowed. See
	// comments for PublishSettings.BufferedByteLimit.
	ErrOverflow = errors.New("pubsublite: client-side publish buffers have overflowed")

	// ErrOversizedMessage indicates that the user published a message over the
	// allowed serialized byte size limit. It is wrapped in another error.
	ErrOversizedMessage = fmt.Errorf("maximum allowed message size is MaxPublishRequestBytes (%d)", MaxPublishRequestBytes)

	// ErrServiceUninitialized indicates that a service (e.g. publisher or
	// subscriber) cannot perform an operation because it is uninitialized.
	ErrServiceUninitialized = errors.New("pubsublite: service must be started")

	// ErrServiceStarting indicates that a service (e.g. publisher or subscriber)
	// cannot perform an operation because it is starting up.
	ErrServiceStarting = errors.New("pubsublite: service is starting up")

	// ErrServiceStopped indicates that a service (e.g. publisher or subscriber)
	// cannot perform an operation because it has stoped or is in the process of
	// stopping.
	ErrServiceStopped = errors.New("pubsublite: service has stopped or is stopping")
)
