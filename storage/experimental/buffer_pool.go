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

package experimental

// This interface is not supported at the moment.
type BufferPool interface {
	// Get retrieves a single chunk of memory. It returns a slice that is
	// optimally sized by the pool, guaranteeing that len(buf) <= maxSize.
	// It returns an error if the underlying allocation fails.
	// Get retrieves a chunk of memory up to maxSize bytes.
	// It blocks/waits if the pool's memory budget is currently exhausted.
	Get(maxSize int) ([]byte, error)

	// TryGet opportunistically retrieves a chunk of memory up to maxSize bytes.
	// It is non-blocking and returns an error immediately if memory is unavailable.
	TryGet(maxSize int) ([]byte, error)

	// Put returns a previously acquired buffer to the pool.
	Put(buf []byte)
}
