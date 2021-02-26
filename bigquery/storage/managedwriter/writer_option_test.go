// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package managedwriter

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/descriptorpb"
)

type testSerializer struct {
}

func (ts *testSerializer) Describe() *descriptorpb.DescriptorProto {
	return nil
}

func (ts *testSerializer) Convert(in interface{}) ([][]byte, error) {
	return nil, fmt.Errorf("unimplemented")
}

func TestWriterOptions(t *testing.T) {

	testCases := []struct {
		desc    string
		options []WriterOption
		want    *ManagedWriter
	}{
		{
			desc:    "WithType",
			options: []WriterOption{WithType(BufferedStream)},
			want: func() *ManagedWriter {
				mw := &ManagedWriter{
					settings: defaultSettings(),
				}
				mw.settings.StreamType = BufferedStream
				return mw
			}(),
		},
		{
			desc:    "WithMaxInflightRequests",
			options: []WriterOption{WithMaxInflightRequests(2)},
			want: func() *ManagedWriter {
				mw := &ManagedWriter{
					settings: defaultSettings(),
				}
				mw.settings.MaxInflightRequests = 2
				return mw
			}(),
		},
		{
			desc:    "WithMaxInflightBytes",
			options: []WriterOption{WithMaxInflightBytes(5)},
			want: func() *ManagedWriter {
				mw := &ManagedWriter{
					settings: defaultSettings(),
				}
				mw.settings.MaxInflightBytes = 5
				return mw
			}(),
		},
		{
			desc:    "WithRowSerializer",
			options: []WriterOption{WithRowSerializer(&testSerializer{})},
			want: func() *ManagedWriter {
				mw := &ManagedWriter{
					settings: defaultSettings(),
				}
				mw.settings.Serializer = &testSerializer{}
				return mw
			}(),
		},
		{
			desc:    "WithTracePrefix",
			options: []WriterOption{WithTracePrefix("foo")},
			want: func() *ManagedWriter {
				mw := &ManagedWriter{
					settings: defaultSettings(),
				}
				mw.settings.TracePrefix = "foo"
				return mw
			}(),
		},
		{
			desc: "multiple",
			options: []WriterOption{
				WithRowSerializer(&testSerializer{}),
				WithType(PendingStream),
				WithMaxInflightBytes(5),
				WithTracePrefix("pre"),
			},
			want: func() *ManagedWriter {
				mw := &ManagedWriter{
					settings: defaultSettings(),
				}
				mw.settings.Serializer = &testSerializer{}
				mw.settings.MaxInflightBytes = 5
				mw.settings.StreamType = PendingStream
				mw.settings.TracePrefix = "pre"
				return mw
			}(),
		},
	}

	for _, tc := range testCases {
		got := &ManagedWriter{
			settings: defaultSettings(),
		}
		for _, o := range tc.options {
			o(got)
		}

		if diff := cmp.Diff(got, tc.want,
			cmp.AllowUnexported(ManagedWriter{}, WriteSettings{})); diff != "" {
			t.Errorf("diff in case (%s):\n%v", tc.desc, diff)
		}
	}
}
