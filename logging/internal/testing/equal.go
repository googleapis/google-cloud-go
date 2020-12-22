/*
Copyright 2017 Google LLC

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

package testing

import (
	"context"
	"fmt"
	"time"
	
	cinternal "cloud.google.com/go/internal"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/golang/protobuf/proto"
)

// TODO rename this file? to common?

// PayloadEqual compares two payloads, assuming they are both proto.Messages or both strings.
func PayloadEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch a := a.(type) {
	case proto.Message:
		return proto.Equal(a, b.(proto.Message))
	case string:
		return a == b.(string)
	default:
		panic(fmt.Sprintf("payloadEqual: unexpected type %T", a))
	}
}

func WaitFor(f func() bool) bool {
	// TODO(shadams): Find a better way to deflake these tests.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	err := cinternal.Retry(ctx,
		gax.Backoff{Initial: time.Second, Multiplier: 2, Max: 30 * time.Second},
		func() (bool, error) { return f(), nil })
	return err == nil
}