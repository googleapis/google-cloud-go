// Copyright 2024 Google LLC
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

// Package priorityqueue provides a generic priority queue with Insert
// and PopMax operations.
package priorityqueue

// PriorityQueue is a generic priority queue with a configurable comparison
// function.
type PriorityQueue[T any] struct {
	cmp func(a, b T) int

	// items holds the queue's items as a binary heap.
	// items[0] is a dummy element that's not used. If the queue has N elements,
	// they are stored at indices 1...N (N == len(items)-1)
	// For an element at index i, its parent is at index i/2, and its children
	// are at indices 2i and 2i+1. The root of the heap is at index 1.
	items []T
}

// New creates a new PriorityQueue, configured with a function that
// compares the priorities of two items a and b; it should return a number > 0
// if the priority of a is higher, 0 if the priorities are equal a number < 0
// otherwise.
func New[T any](cmp func(a, b T) int) *PriorityQueue[T] {
	return &PriorityQueue[T]{cmp: cmp, items: make([]T, 1)}
}

// Len returns the length (number of items) of the priority queue.
func (pq *PriorityQueue[T]) Len() int {
	return len(pq.items) - 1
}

// Insert inserts a new element into the priority queue.
func (pq *PriorityQueue[T]) Insert(elem T) {
	pq.items = append(pq.items, elem)
	pq.siftup(len(pq.items) - 1)
}

// PopMax returns the element with the maximal priority in the queue, and
// removes it from the queue. Warning: to maintain a clean API, PopMax panics
// if the queue is empty. Make sure to check Len() first.
func (pq *PriorityQueue[T]) PopMax() T {
	if len(pq.items) < 2 {
		panic("popping from empty priority queue")
	}
	maxItem := pq.items[1]
	pq.items[1] = pq.items[len(pq.items)-1]
	pq.items = pq.items[:len(pq.items)-1]
	pq.siftdown()
	return maxItem
}

func (pq *PriorityQueue[T]) siftup(n int) {
	i := n
	for {
		if i == 1 {
			// Reached root, we're done.
			return
		}
		// p is the index of i's parent
		// if p parent has a higher priority than i, we're done.
		p := i / 2
		if pq.cmp(pq.items[p], pq.items[i]) >= 0 {
			return
		}
		pq.items[i], pq.items[p] = pq.items[p], pq.items[i]
		i = p
	}
}

func (pq *PriorityQueue[T]) siftdown() {
	i := 1
	for {
		c := 2 * i
		if c >= len(pq.items) {
			return
		}
		// c is not out of bounds, so it's the index of the left child of i

		// Figure out the child index with the maximal priority
		maxChild := c
		if c+1 < len(pq.items) {
			// c+1 is not out of bounds, so it's the index of the right child of i
			if pq.cmp(pq.items[c+1], pq.items[c]) > 0 {
				maxChild = c + 1
			}
		}
		if pq.cmp(pq.items[i], pq.items[maxChild]) >= 0 {
			// i has higher priority than either child, so we're done.
			return
		}

		pq.items[i], pq.items[maxChild] = pq.items[maxChild], pq.items[i]
		i = maxChild
	}
}
