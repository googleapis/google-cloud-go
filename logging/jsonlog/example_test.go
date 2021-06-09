// Copyright 2021 Google LLC
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

package jsonlog_test

import (
	"io"
	"net/http"
	"os"

	"cloud.google.com/go/logging/jsonlog"
)

func ExampleNewLogger() {
	l, err := jsonlog.NewLogger("projects/PROJECT_ID")
	if err != nil {
		// TODO: handle error.
	}
	l.Infof("Hello World!")
}

func ExampLogger_WithRequest() {
	var req *http.Request
	l, err := jsonlog.NewLogger("projects/PROJECT_ID")
	if err != nil {
		// TODO: handle error.
	}
	// Create a Logger with additional information pulled from the current
	// request context.
	l = l.WithRequest(req)
	l.Infof("Hello World!")
}

func ExampleLogger_WithLabels() {
	l, err := jsonlog.NewLogger("projects/PROJECT_ID")
	if err != nil {
		// TODO: handle error.
	}
	l.Infof("Hello World!")

	// Create a logger that always provides additional context by adding labels
	// to all logged messages.
	l2 := l.WithLabels(map[string]string{"foo": "bar"})
	l2.Infof("Hello World, with more context!")
}

func ExampleWithWriter_multiwriter() {
	// Create a new writer that also logs messages to a second location
	w := io.MultiWriter(os.Stderr, os.Stdout)
	l, err := jsonlog.NewLogger("projects/PROJECT_ID", jsonlog.WithWriter(w))
	if err != nil {
		// TODO: handle error.
	}
	l.Infof("Hello World!")
	// Output: {"message":"Hello World!","severity":"INFO"}
}
