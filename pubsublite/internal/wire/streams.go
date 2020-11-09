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
	"io"
	"reflect"
	"sync"
	"time"

	"google.golang.org/grpc"

	gax "github.com/googleapis/gax-go/v2"
)

// streamStatus is the status of a retryableStream. A stream starts off
// uninitialized. While it is active, it can transition between reconnecting and
// connected due to retryable errors. When a permanent error occurs, the stream
// is terminated and cannot be reconnected.
type streamStatus int

const (
	streamUninitialized streamStatus = 0
	streamReconnecting  streamStatus = 1
	streamConnected     streamStatus = 2
	streamTerminated    streamStatus = 3
)

// streamHandler provides hooks for different Pub/Sub Lite streaming APIs
// (e.g. publish, subscribe, streaming cursor, etc.) to use retryableStream.
// All Pub/Sub Lite streaming APIs implement a similar handshaking protocol,
// where an initial request and response must be transmitted before other
// requests can be sent over the stream.
//
// streamHandler methods must not be called while holding retryableStream.mu in
// order to prevent the streamHandler calling back into the retryableStream and
// deadlocking.
type streamHandler interface {
	// newStream implementations must create the client stream with the given
	// (cancellable) context.
	newStream(context.Context) (grpc.ClientStream, error)
	initialRequest() interface{}
	validateInitialResponse(interface{}) error

	// onStreamStatusChange is used to notify stream handlers when the stream has
	// changed state. In particular, the `streamTerminated` state must be handled.
	// retryableStream.Error() returns the error that caused the stream to
	// terminate. Stream handlers should perform any necessary reset of state upon
	// `streamConnected`.
	onStreamStatusChange(streamStatus)
	// onResponse forwards a response received on the stream to the stream
	// handler.
	onResponse(interface{})
}

// retryableStream is a wrapper around a bidirectional gRPC client stream to
// handle automatic reconnection when the stream breaks.
//
// A retryableStream cycles between the following goroutines:
//   Start() --> reconnect() <--> listen()
// terminate() can be called at any time, either by the client to force stream
// closure, or as a result of an unretryable error.
//
// Safe to call capitalized methods from multiple goroutines. All other methods
// are private implementation.
type retryableStream struct {
	// Immutable after creation.
	ctx          context.Context
	handler      streamHandler
	responseType reflect.Type
	timeout      time.Duration

	// Guards access to fields below.
	mu sync.Mutex

	// The current connected stream.
	stream grpc.ClientStream
	// Function to cancel the current stream (which may be reconnecting).
	cancelStream context.CancelFunc
	status       streamStatus
	finalErr     error
}

// newRetryableStream creates a new retryable stream wrapper. `timeout` is the
// maximum duration for reconnection. `responseType` is the type of the response
// proto received on the stream.
func newRetryableStream(ctx context.Context, handler streamHandler, timeout time.Duration, responseType reflect.Type) *retryableStream {
	return &retryableStream{
		ctx:          ctx,
		handler:      handler,
		responseType: responseType,
		timeout:      timeout,
	}
}

// Start establishes a stream connection. It is a no-op if the stream has
// already started.
func (rs *retryableStream) Start() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.status != streamUninitialized {
		return
	}
	go rs.reconnect()
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
			go rs.reconnect()
		default:
			rs.mu.Unlock() // terminate acquires the mutex.
			rs.terminate(err)
			return
		}
	}

	rs.mu.Unlock()
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

func (rs *retryableStream) setCancel(cancel context.CancelFunc) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.cancelStream = cancel
}

// reconnect attempts to establish a valid connection with the server. Due to
// the potential high latency, initNewStream() should not be done while holding
// retryableStream.mu. Hence we need to handle the stream being force terminated
// during reconnection.
//
// Intended to be called in a goroutine. It ends once the connection has been
// established or the stream terminated.
func (rs *retryableStream) reconnect() {
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
		return true
	}
	if !canReconnect() {
		return
	}
	rs.handler.onStreamStatusChange(streamReconnecting)

	newStream, cancelFunc, err := rs.initNewStream()
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
		rs.cancelStream = cancelFunc
		go rs.listen(newStream)
		return true
	}
	if !connected() {
		return
	}
	rs.handler.onStreamStatusChange(streamConnected)
}

func (rs *retryableStream) initNewStream() (newStream grpc.ClientStream, cancelFunc context.CancelFunc, err error) {
	r := newStreamRetryer(rs.timeout)
	for {
		backoff, shouldRetry := func() (time.Duration, bool) {
			defer func() {
				if err != nil && cancelFunc != nil {
					cancelFunc()
					cancelFunc = nil
					newStream = nil
				}
			}()

			var cctx context.Context
			cctx, cancelFunc = context.WithCancel(rs.ctx)
			// Store the cancel func to quickly cancel reconnecting if the stream is
			// terminated.
			rs.setCancel(cancelFunc)

			newStream, err = rs.handler.newStream(cctx)
			if err != nil {
				return r.RetryRecv(err)
			}
			if err = newStream.SendMsg(rs.handler.initialRequest()); err != nil {
				return r.RetrySend(err)
			}
			response := reflect.New(rs.responseType).Interface()
			if err = newStream.RecvMsg(response); err != nil {
				return r.RetryRecv(err)
			}
			if err = rs.handler.validateInitialResponse(response); err != nil {
				// An unexpected initial response from the server is a permanent error.
				return 0, false
			}

			// We have a valid connection and should break from the outer loop.
			return 0, false
		}()

		if !shouldRetry {
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
//
// Intended to be called in a goroutine. It ends when recvStream has closed.
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
		if err != nil {
			if isRetryableRecvError(err) {
				go rs.reconnect()
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

	if rs.status == streamTerminated {
		return
	}
	rs.status = streamTerminated
	rs.finalErr = err
	rs.unsafeClearStream()

	// terminate can be called from within a streamHandler method with a lock
	// held. So notify from a goroutine to prevent deadlock.
	go rs.handler.onStreamStatusChange(streamTerminated)
}
