/*
Copyright 2021 Google LLC

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

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cloud.google.com/go/internal/testutil"
)

func captureStdout(f func()) string {
	/*
	   Capture standard output to facilitate testing code that prints

	   or useless print output in running tests.
	*/
	saved := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "Pipe failed"
	}
	os.Stdout = w
	defer func() { os.Stdout = saved }()

	outC := make(chan string)
	// https://stackoverflow.com/questions/10473800/in-go-how-do-i-capture-stdout-of-a-function-into-a-string
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	f()

	w.Close()
	return <-outC
}

func assertEqual(t *testing.T, got, want interface{}) {
	if !testutil.Equal(got, want) {
		_, fpath, lno, ok := runtime.Caller(1)
		if ok {
			_, fname := filepath.Split(fpath)
			t.Errorf("%s:%d: Didn't match:\n%s", fname, lno, got)
		} else {
			t.Errorf("Didn't match:\n%s", got)
		}
	}
}

func assertNoError(t *testing.T, err error) {
	if err != nil {
		_, fpath, lno, ok := runtime.Caller(1)
		if ok {
			_, fname := filepath.Split(fpath)
			t.Fatalf("%s:%d: %s", fname, lno, err)
		} else {
			t.Fatal(err)
		}
	}
}
