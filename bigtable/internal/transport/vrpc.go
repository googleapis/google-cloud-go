// Copyright 2026 Google LLC
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

package internal

import (
	"context"
)

// CallContext represents the execution context of a virtual RPC invocation.
type CallContext interface {
	context.Context
	// Method returns the name of the virtual RPC method.
	Method() string
	// Attempt returns the current attempt number (1-indexed).
	Attempt() int
}

// Handler represents the core execution function of a virtual RPC downstream invocation.
type Handler func(ctx CallContext, req interface{}) (interface{}, error)

// Interceptor represents a decorator middleware that intercepts a virtual RPC downstream invocation.
type Interceptor func(ctx CallContext, req interface{}, handler Handler) (interface{}, error)

// VRpcListener receives notifications of vRPC attempt lifecycles.
type VRpcListener interface {
	// OnAttemptStart is called before a new attempt is executed.
	OnAttemptStart(ctx CallContext)
	// OnAttemptComplete is called after an attempt completes (with success or error).
	OnAttemptComplete(ctx CallContext, err error)
}

type callContext struct {
	context.Context
	method  string
	attempt int
}

func (c *callContext) Method() string { return c.method }
func (c *callContext) Attempt() int   { return c.attempt }

// NewCallContext creates a new CallContext with initial attempt set to 1.
func NewCallContext(ctx context.Context, method string) CallContext {
	return &callContext{
		Context: ctx,
		method:  method,
		attempt: 1,
	}
}
