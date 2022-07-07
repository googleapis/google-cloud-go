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
	"context"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gax "github.com/googleapis/gax-go/v2"
)

// streamStatus is the status of a retryableStream. A stream starts off
// uninitialized. While it is active, it can transition between reconnecting and
// connected due to retryable errors. When a permanent error occurs, the stream
// is terminated and cannot be reconnected.
type streamStatus int

const (
	streamUninitialized streamStatus = iota
	streamReconnecting
	streamResetState
	streamConnected
	streamTerminated
)

const (
	// Abort a stream initialization attempt after this duration to mitigate delays.
	defaultStreamInitTimeout = 2 * time.Minute

	// Reconnect a stream if it has been idle for this duration.
	defaultStreamIdleTimeout = 2 * time.Minute
)

var errStreamInitTimeout = status.Error(codes.DeadlineExceeded, "pubsublite: stream initialization timed out")

type initialResponseRequired bool
type notifyReset bool

func streamIdleTimeout(userTimeout time.Duration) time.Duration {
	return minDuration(userTimeout/2, defaultStreamIdleTimeout)
}

// streamHandler provides hooks for different Pub/Sub Lite streaming APIs
// (e.g. publish, subscribe, streaming cursor, etc.) to use retryableStream.
// All Pub/Sub Lite streaming APIs implement a similar handshaking protocol,
// where an initial request and response must be transmitted before other
// requests can be sent over the stream.
//
// streamHandler methods must not be called while holding retryableStream.mu in
// order to prevent the streamHandler calling back into the retryableStream and
// deadlocking.
//
// If any streamHandler method implementations block, this will block the
// retryableStream.connectStream goroutine processing the underlying stream.
type streamHandler interface {
	// newStream implementations must create the client stream with the given
	// (cancellable) context.
	newStream(context.Context) (grpc.ClientStream, error)
	// initialRequest should return the initial request and whether an initial
	// response is expected.
	initialRequest() (interface{}, initialResponseRequired)
	validateInitialResponse(interface{}) error

	// onStreamStatusChange is used to notify stream handlers when the stream has
	// changed state.
	// - A `streamReconnecting` status change is fired before attempting to
	//   connect a new stream.
	// - A `streamResetState` status change may be fired if the stream should
	//   reset its state (due to receipt of the RESET signal from the server).
	// - A `streamConnected` status change is fired when the stream is
	//   successfully connected.
	// These are followed by onResponse() calls when responses are received from
	// the server. These events are guaranteed to occur in this order.
	//
	// A final `streamTerminated` status change is fired when a permanent error
	// occurs. retryableStream.Error() returns the error that caused the stream to
	// terminate.
	onStreamStatusChange(streamStatus)
	// onResponse forwards a response received on the stream to the stream
	// handler.
	onResponse(interface{})
}

// retryableStream is a wrapper around a bidirectional gRPC client stream to
// handle automatic reconnection when the stream breaks.
//
// The connectStream() goroutine handles each stream connection. terminate() can
// be called at any time, either by the client to force stream closure, or as a
// result of an unretryable error.
//
// Safe to call capitalized methods from multiple goroutines. All other methods
// are private implementation.
type retryableStream struct {
	// Immutable after creation.
	ctx            context.Context
	handler        streamHandler
	responseType   reflect.Type
	connectTimeout time.Duration
	initTimeout    time.Duration
	idleTimer      *streamIdleTimer

	// Guards access to fields below.
	mu sync.Mutex

	// The current connected stream.
	stream grpc.ClientStream
	// Function to cancel the current stream (which may be reconnecting).
	cancelStream context.CancelFunc
	status       streamStatus
	finalErr     error
}

// newRetryableStream creates a new retryable stream wrapper.
// `connectTimeout` is the maximum duration for reconnection, after which the
// stream will be terminated. Streams are reconnected if idle for `idleTimeout`.
// `responseType` is the type of the response proto received on the stream.
func newRetryableStream(ctx context.Context, handler streamHandler, connectTimeout, idleTimeout time.Duration, responseType reflect.Type) *retryableStream {
	// Retry initialization before the reconnection timeout.
	initTimeout := minDuration(connectTimeout/2, defaultStreamInitTimeout)
	rs := &retryableStream{
		ctx:            ctx,
		handler:        handler,
		responseType:   responseType,
		connectTimeout: connectTimeout,
		initTimeout:    initTimeout,
	}
	rs.idleTimer = newStreamIdleTimer(idleTimeout, rs.onStreamIdle)
	return rs
}

// Start establishes a stream connection. It is a no-op if the stream has
// already started.
func (rs *retryableStream) Start() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.status == streamUninitialized {
		go rs.connectStream(notifyReset(false))
	}
}

// Stop gracefully closes the stream without error.
func (rs *retryableStream) Stop() {
	rs.terminate(nil)
}

// Send attempts to send the request to the underlying stream and returns true
// if successfully sent. Returns false if an error occurred or a reconnection is
// in progress.
func (rs *retryableStream) Send(request interface{}) (sent bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.stream != nil {
		err := rs.stream.SendMsg(request)
		// Note: if SendMsg returns an error, the stream is aborted.
		switch {
		case err == nil:
			sent = true
		case err == io.EOF:
			// If SendMsg returns io.EOF, RecvMsg will return the status of the
			// stream. Nothing to do here.
			break
		case isRetryableSendError(err):
			go rs.connectStream(notifyReset(false))
		default:
			rs.unsafeTerminate(err)
		}
	}
	return
}

// Status returns the current status of the retryable stream.
func (rs *retryableStream) Status() streamStatus {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.status
}

// Error returns the error that caused the stream to terminate. Can be nil if it
// was initiated by Stop().
func (rs *retryableStream) Error() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.finalErr
}

func (rs *retryableStream) currentStream() grpc.ClientStream {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.stream
}

// unsafeClearStream must be called with the retryableStream.mu locked.
func (rs *retryableStream) unsafeClearStream() {
	if rs.cancelStream != nil {
		// If the stream did not already abort due to error, this will abort it.
		rs.cancelStream()
		rs.cancelStream = nil
	}
	if rs.stream != nil {
		rs.stream = nil
	}
}

func (rs *retryableStream) onStreamIdle() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Invalidate the current stream handle so that subsequent messages and errors
	// are discarded.
	rs.unsafeClearStream()
	go rs.connectStream(notifyReset(false))
}

func (rs *retryableStream) newStreamContext() (ctx context.Context, cancel context.CancelFunc) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.unsafeClearStream()
	ctx, cancel = context.WithCancel(rs.ctx)
	rs.cancelStream = cancel
	return
}

// connectStream attempts to establish a valid connection with the server. Due
// to the potential high latency, initNewStream() should not be done while
// holding retryableStream.mu. Hence we need to handle the stream being force
// terminated during reconnection.
//
// Intended to be called in a goroutine. It ends once the client stream closes.
func (rs *retryableStream) connectStream(notifyReset notifyReset) {
	canReconnect := func() bool {
		rs.mu.Lock()
		defer rs.mu.Unlock()

		if rs.status == streamReconnecting {
			// There can only be 1 goroutine reconnecting.
			return false
		}
		if rs.status == streamTerminated {
			return false
		}
		rs.status = streamReconnecting
		rs.unsafeClearStream()
		rs.idleTimer.Stop()
		return true
	}
	if !canReconnect() {
		return
	}

	rs.handler.onStreamStatusChange(streamReconnecting)
	if notifyReset {
		rs.handler.onStreamStatusChange(streamResetState)
	}
	// Check whether handler terminated stream before reconnecting.
	if rs.Status() == streamTerminated {
		return
	}

	newStream, err := rs.initNewStream()
	if err != nil {
		rs.terminate(err)
		return
	}

	connected := func() bool {
		rs.mu.Lock()
		defer rs.mu.Unlock()

		if rs.status == streamTerminated {
			rs.unsafeClearStream()
			return false
		}
		rs.status = streamConnected
		rs.stream = newStream
		rs.idleTimer.Restart()
		return true
	}
	if !connected() {
		return
	}

	rs.handler.onStreamStatusChange(streamConnected)
	rs.listen(newStream)
}

func (rs *retryableStream) newInitTimer(cancelFunc func()) *requestTimer {
	return newRequestTimer(rs.initTimeout, cancelFunc, errStreamInitTimeout)
}

func (rs *retryableStream) initNewStream() (newStream grpc.ClientStream, err error) {
	var cancelFunc context.CancelFunc
	r := newStreamRetryer(rs.connectTimeout)
	for {
		backoff, shouldRetry := func() (time.Duration, bool) {
			var cctx context.Context
			cctx, cancelFunc = rs.newStreamContext()
			// Bound the duration of the stream initialization attempt.
			it := rs.newInitTimer(cancelFunc)
			defer it.Stop()

			newStream, err = rs.handler.newStream(cctx)
			if err = it.ResolveError(err); err != nil {
				return r.RetryRecv(err)
			}
			initReq, needsResponse := rs.handler.initialRequest()
			if err = it.ResolveError(newStream.SendMsg(initReq)); err != nil {
				return r.RetrySend(err)
			}
			if needsResponse {
				response := reflect.New(rs.responseType).Interface()
				if err = it.ResolveError(newStream.RecvMsg(response)); err != nil {
					if isStreamResetSignal(err) {
						rs.handler.onStreamStatusChange(streamResetState)
					}
					return r.RetryRecv(err)
				}
				if err = rs.handler.validateInitialResponse(response); err != nil {
					// An unexpected initial response from the server is a permanent error.
					return 0, false
				}
			}

			// If the init timer fired due to a race, the stream would be unusable.
			it.Stop()
			if err = it.ResolveError(nil); err != nil {
				return r.RetryRecv(err)
			}

			// We have a valid connection and should break from the outer loop.
			return 0, false
		}()

		if (shouldRetry || err != nil) && cancelFunc != nil {
			// Ensure that streams aren't leaked.
			cancelFunc()
			cancelFunc = nil
			newStream = nil
		}
		if !shouldRetry || rs.Status() == streamTerminated {
			break
		}
		if r.ExceededDeadline() {
			err = fmt.Errorf("%v: %w", err, ErrBackendUnavailable)
			break
		}
		if err = gax.Sleep(rs.ctx, backoff); err != nil {
			break
		}
	}
	return
}

// listen receives responses from the current stream. It initiates reconnection
// upon retryable errors or terminates the stream upon permanent error.
func (rs *retryableStream) listen(recvStream grpc.ClientStream) {
	for {
		response := reflect.New(rs.responseType).Interface()
		err := recvStream.RecvMsg(response)

		// If the current stream has changed while listening, any errors or messages
		// received now are obsolete. Discard and end the goroutine. Assume the
		// stream has been cancelled elsewhere.
		if rs.currentStream() != recvStream {
			break
		}
		rs.idleTimer.Restart()
		if err != nil {
			if isRetryableRecvError(err) {
				go rs.connectStream(notifyReset(isStreamResetSignal(err)))
			} else {
				rs.terminate(err)
			}
			break
		}
		rs.handler.onResponse(response)
	}
}

// terminate forces the stream to terminate with the given error (can be nil)
// Is a no-op if the stream has already terminated.
func (rs *retryableStream) terminate(err error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.unsafeTerminate(err)
}

func (rs *retryableStream) unsafeTerminate(err error) {
	if rs.status == streamTerminated {
		return
	}
	rs.status = streamTerminated
	rs.finalErr = err
	rs.idleTimer.Shutdown()
	rs.unsafeClearStream()

	// terminate can be called from within a streamHandler method with a lock
	// held. So notify from a goroutine to prevent deadlock.
	go rs.handler.onStreamStatusChange(streamTerminated)
}
