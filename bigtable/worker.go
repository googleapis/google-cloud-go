/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"fmt"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/metadata"
)

const defaultPoolSize = 5

// TODO: Better structure for this
type job struct {
	// TODO: Support other streams
	stream   *btpb.Bigtable_ReadRowsClient
	mt       builtinMetricsTracer
	headerMD metadata.MD
	handler  func(*builtinMetricsTracer, *btpb.Bigtable_ReadRowsClient, metadata.MD)
}

type pool struct {
	size int
	jobs chan job
}

func worker(id int, jobs <-chan job) {
	fmt.Println("worker", id, "started")
	defer fmt.Println("worker", id, "stopped")
	for j := range jobs {
		fmt.Println("worker", id, "processing job", j.mt.method)
		j.handler(&j.mt, j.stream, j.headerMD)
	}
}

func newPool(size int) *pool {
	if size <= 0 {
		size = defaultPoolSize
	}
	p := &pool{
		size: size,
		jobs: make(chan job, size),
	}
	for w := 1; w <= size; w++ {
		go worker(w, p.jobs)
	}
	return p
}

func (p *pool) addJob(j *job) {
	p.jobs <- *j
}
