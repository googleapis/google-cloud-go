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

package jsonlog

import (
	"bytes"
	"encoding/json"
	"testing"

	"cloud.google.com/go/logging"
)

func TestOnErrorHook(t *testing.T) {
	var i *int = new(int)
	fn := func(error) {
		*i++
	}
	l, err := NewLogger("projects/test", OnErrorHook(fn))
	if err != nil {
		t.Fatalf("unable to create logger: %v", err)
	}
	l.Log(logging.Entry{
		Payload: false,
	})
	if *i != 1 {
		t.Fatalf("got %d, want %d", *i, 1)
	}
}

func TestWithWriter(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	l, err := NewLogger("projects/test", WithWriter(buf))
	if err != nil {
		t.Fatalf("unable to create logger: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("buf.Len() = %d, want 0", buf.Len())
	}
	l.Infof("Test")
	if buf.Len() == 0 {
		t.Fatalf("buf.Len() = %d, want >0", buf.Len())
	}
}

func TestCommonLabels(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	l, err := NewLogger("projects/test", CommonLabels(map[string]string{"foo": "bar"}))
	if err != nil {
		t.Fatalf("unable to create logger: %v", err)
	}
	l.w = buf
	l.Infof("Test")
	e := &entry{}
	if err := json.Unmarshal(buf.Bytes(), e); err != nil {
		t.Fatalf("unable to unmarshal: %v", err)
	}
	if e.Labels["foo"] != "bar" {
		t.Fatalf("le.Labels[\"foo\"] = %q, want \"bar\"", e.Labels["foo"])
	}
}
