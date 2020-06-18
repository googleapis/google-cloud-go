// Copyright 2019 Google LLC
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

package scheduler_test

import (
	"fmt"
	"reflect"
	"testing"

	"cloud.google.com/go/pubsub/internal/scheduler"
)

const pubSchedulerWorkers = 100

func BenchmarkPublisher_Unkeyed(b *testing.B) {
	wait := make(chan struct{}, b.N)
	ps := scheduler.NewPublishScheduler(pubSchedulerWorkers, func(bundle interface{}) {
		nlen := reflect.ValueOf(bundle).Len()
		for i := 0; i < nlen; i++ {
			wait <- struct{}{}
		}
	})
	go func() {
		for i := 0; i < b.N; i++ {
			if err := ps.Add("", fmt.Sprintf("item_%d", i), 1); err != nil {
				b.Error(err)
			}
		}
	}()
	for j := 0; j < b.N; j++ {
		<-wait
	}
}

func BenchmarkPublisher_SingleKey(b *testing.B) {
	wait := make(chan struct{}, b.N)
	ps := scheduler.NewPublishScheduler(pubSchedulerWorkers, func(bundle interface{}) {
		nlen := reflect.ValueOf(bundle).Len()
		for i := 0; i < nlen; i++ {
			wait <- struct{}{}
		}
	})
	go func() {
		for i := 0; i < b.N; i++ {
			if err := ps.Add("some-key", fmt.Sprintf("item_%d", i), 1); err != nil {
				b.Error(err)
			}
		}
	}()
	for j := 0; j < b.N; j++ {
		<-wait
	}
}
