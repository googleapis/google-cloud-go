// Copyright 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package speech

import (
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1beta1"
	longrunningpb "google.golang.org/genproto/googleapis/longrunning"
)

import (
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockSpeech struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ speechpb.SpeechServer = &mockSpeech{}

func (s *mockSpeech) SyncRecognize(_ context.Context, req *speechpb.SyncRecognizeRequest) (*speechpb.SyncRecognizeResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*speechpb.SyncRecognizeResponse), nil
}

func (s *mockSpeech) AsyncRecognize(_ context.Context, req *speechpb.AsyncRecognizeRequest) (*longrunningpb.Operation, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*longrunningpb.Operation), nil
}

func (s *mockSpeech) StreamingRecognize(stream speechpb.Speech_StreamingRecognizeServer) error {
	if s.err != nil {
		return s.err
	}

	ch := make(chan error, 2)
	go func() {
		for {
			if req, err := stream.Recv(); err == io.EOF {
				ch <- nil
				return
			} else if err != nil {
				ch <- err
				return
			} else {
				s.reqs = append(s.reqs, req)
			}
		}
	}()
	go func() {
		for _, v := range s.resps {
			if err := stream.Send(v.(*speechpb.StreamingRecognizeResponse)); err != nil {
				ch <- err
				return
			}
		}
		ch <- nil
	}()

	// Doesn't really matter which one we get.
	err := <-ch
	if err2 := <-ch; err == nil {
		err = err2
	}
	return err
}
