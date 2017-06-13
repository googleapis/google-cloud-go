// Copyright 2017 Google Inc. All Rights Reserved.
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

package rpcreplay

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	rpb "cloud.google.com/go/internal/rpcreplay/proto/rpcreplay"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecordIO(t *testing.T) {
	buf := &bytes.Buffer{}
	want := []byte{1, 2, 3}
	if err := writeRecord(buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := readRecord(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestHeaderIO(t *testing.T) {
	buf := &bytes.Buffer{}
	want := []byte{1, 2, 3}
	if err := writeHeader(buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := readHeader(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// readHeader errors
	for _, contents := range []string{"", "badmagic", "gRPCReplay"} {
		if _, err := readHeader(bytes.NewBufferString(contents)); err == nil {
			t.Errorf("%q: got nil, want error", contents)
		}
	}
}

func TestEntryIO(t *testing.T) {
	for i, want := range []*entry{
		{
			kind:     rpb.Entry_REQUEST,
			method:   "method",
			msg:      message{msg: &rpb.Entry{}},
			refIndex: 7,
		},
		{
			kind:     rpb.Entry_RESPONSE,
			method:   "method",
			msg:      message{err: status.Error(codes.NotFound, "not found")},
			refIndex: 8,
		},
		{
			kind:     rpb.Entry_RECV,
			method:   "method",
			msg:      message{err: io.EOF},
			refIndex: 3,
		},
	} {
		buf := &bytes.Buffer{}
		if err := writeEntry(buf, want); err != nil {
			t.Fatal(err)
		}
		got, err := readEntry(buf)
		if err != nil {
			t.Fatal(err)
		}
		if !got.equal(want) {
			t.Errorf("#%d: got %v, want %v", i, got, want)
		}
	}
}
