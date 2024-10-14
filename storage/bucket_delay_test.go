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

package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBucketDelay(t *testing.T) {
	b, err := newBucketDelay(0.99, 1.5, 100*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	if err != nil {
		t.Errorf("while creating bucketDelay: %v", err)
	}

	// Test increase and getValue
	b.increase("bucket1")
	delay1 := b.getValue("bucket1")
	if delay1 <= 100*time.Millisecond {
		t.Errorf("Expected delay for bucket1 to be > 100ms after increase, got %v", delay1)
	}

	// Test decrease and getValue
	b.decrease("bucket1")
	delay2 := b.getValue("bucket1")
	if delay2 >= delay1 {
		t.Errorf("Expected delay for bucket1 to be < %v after decrease, got %v", delay1, delay2)
	}

	// Test update with latency > current delay
	b.update("bucket2", 200*time.Millisecond)
	delay3 := b.getValue("bucket2")
	if delay3 <= 100*time.Millisecond {
		t.Errorf("Expected delay for bucket2 to be > 100ms after update with higher latency, got %v", delay3)
	}

	// Test update with latency < current delay
	b.update("bucket2", 50*time.Millisecond)
	delay4 := b.getValue("bucket2")
	if delay4 >= delay3 {
		t.Errorf("Expected delay for bucket2 to be < %v after update with lower latency, got %v", delay3, delay4)
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bucketName := fmt.Sprintf("bucket%d", i%3) // Use only 3 buckets to increase concurrency
			b.increase(bucketName)
			b.decrease(bucketName)
			b.update(bucketName, time.Duration(i)*time.Millisecond)
		}(i)
	}
	wg.Wait()
}
