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

package support

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestTransformMapValues(t *testing.T) {
	var from map[string]int
	got := pvTransformMapValues(from, strconv.Itoa)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	from = map[string]int{"one": 1, "two": 2}
	got = pvTransformMapValues(from, strconv.Itoa)
	want := map[string]string{"one": "1", "two": "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAPIError(t *testing.T) {
	const (
		code   = 3 // gRPC "invalid argument"
		msg    = "message"
		reason = "reason"
	)
	ei := &errdetails.ErrorInfo{Reason: reason}
	pbany, err := anypb.New(ei)
	if err != nil {
		t.Fatal(err)
	}
	s := &spb.Status{
		Code:    code,
		Message: msg,
		Details: []*anypb.Any{pbany},
	}

	ae := pvAPIErrorFromProto(s)
	if ae == nil {
		t.Fatal("got nil")
	}
	gs := ae.GRPCStatus()
	if g := gs.Code(); g != code {
		t.Errorf("got %d, want %d", g, code)
	}
	if g := gs.Message(); g != msg {
		t.Errorf("got %q, want %q", g, msg)
	}
	if g := ae.Reason(); g != reason {
		t.Errorf("got %q, want %q", g, reason)
	}

	gps := pvAPIErrorToProto(ae)
	if !cmp.Equal(gps, s, cmpopts.IgnoreUnexported(spb.Status{}, anypb.Any{})) {
		t.Errorf("\ngot  %s\nwant %s", gps, s)
	}
}

func TestPanic(t *testing.T) {
	_, err := mapToStructErr(map[string]any{"c": make(chan int)})
	if err == nil {
		t.Fatal("got nil, want error")
	}
	t.Logf("%s", err)
}

func mapToStructErr(m map[string]any) (_ *structpb.Struct, err error) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(pvPanic); ok {
				err = r.(error)
			} else {
				panic(r)
			}
		}
	}()
	return pvMapToStructPB(m), nil
}
