// Copyright 2023 Google LLC
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

package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"
)

func warmupW1R3(ctx context.Context, opts *benchmarkOptions) error {
	// Return immediately if warmup duration is zero.
	if opts.warmup == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	warmupGroup, ctx := errgroup.WithContext(ctx)
	warmupGroup.SetLimit(runtime.NumCPU())

	for deadline := time.Now().Add(opts.warmup); time.Now().Before(deadline); {
		warmupGroup.Go(func() error {
			benchmark := &w1r3{opts: opts, bucketName: opts.bucket, isWarmup: true}

			if err := benchmark.setup(ctx); err != nil {
				return fmt.Errorf("warmup setup failed: %v", err)
			}
			if err := benchmark.run(ctx); err != nil {
				return fmt.Errorf("warmup run failed: %v", err)
			}
			if err := benchmark.cleanup(); err != nil {
				return fmt.Errorf("warmup cleanup failed: %v", err)
			}
			return nil
		})
	}

	return warmupGroup.Wait()
}
