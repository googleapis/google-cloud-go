/*
Copyright 2017 Google Inc. All Rights Reserved.

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

package spanner

import (
	"reflect"
	"testing"

	proto3 "github.com/golang/protobuf/ptypes/struct"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

func TestKeySets(t *testing.T) {
	int1 := intProto(1)
	int2 := intProto(2)
	int3 := intProto(3)
	int4 := intProto(4)
	for i, test := range []struct {
		ks        KeySet
		wantProto *sppb.KeySet
	}{
		{
			KeySets(),
			&sppb.KeySet{},
		},
		{
			Key{4},
			&sppb.KeySet{
				Keys: []*proto3.ListValue{listValueProto(int4)},
			},
		},
		{
			AllKeys(),
			&sppb.KeySet{All: true},
		},
		{
			KeySets(Key{1, 2}, Key{3, 4}),
			&sppb.KeySet{
				Keys: []*proto3.ListValue{
					listValueProto(int1, int2),
					listValueProto(int3, int4),
				},
			},
		},
		{
			KeyRange{Key{1}, Key{2}, ClosedOpen},
			&sppb.KeySet{Ranges: []*sppb.KeyRange{
				&sppb.KeyRange{
					&sppb.KeyRange_StartClosed{listValueProto(int1)},
					&sppb.KeyRange_EndOpen{listValueProto(int2)},
				},
			}},
		},
		{
			Key{2}.AsPrefix(),
			&sppb.KeySet{Ranges: []*sppb.KeyRange{
				&sppb.KeyRange{
					&sppb.KeyRange_StartClosed{listValueProto(int2)},
					&sppb.KeyRange_EndClosed{listValueProto(int2)},
				},
			}},
		},
		{
			KeySets(
				KeyRange{Key{1}, Key{2}, ClosedClosed},
				KeyRange{Key{3}, Key{4}, OpenClosed},
			),
			&sppb.KeySet{
				Ranges: []*sppb.KeyRange{
					&sppb.KeyRange{
						&sppb.KeyRange_StartClosed{listValueProto(int1)},
						&sppb.KeyRange_EndClosed{listValueProto(int2)},
					},
					&sppb.KeyRange{
						&sppb.KeyRange_StartOpen{listValueProto(int3)},
						&sppb.KeyRange_EndClosed{listValueProto(int4)},
					},
				},
			},
		},
		{
			KeySets(
				Key{1},
				KeyRange{Key{2}, Key{3}, ClosedClosed},
				KeyRange{Key{4}, Key{5}, OpenClosed},
				KeySets(),
				Key{6}),
			&sppb.KeySet{
				Keys: []*proto3.ListValue{
					listValueProto(int1),
					listValueProto(intProto(6)),
				},
				Ranges: []*sppb.KeyRange{
					&sppb.KeyRange{
						&sppb.KeyRange_StartClosed{listValueProto(int2)},
						&sppb.KeyRange_EndClosed{listValueProto(int3)},
					},
					&sppb.KeyRange{
						&sppb.KeyRange_StartOpen{listValueProto(int4)},
						&sppb.KeyRange_EndClosed{listValueProto(intProto(5))},
					},
				},
			},
		},
		{
			KeySets(
				Key{1},
				KeyRange{Key{2}, Key{3}, ClosedClosed},
				AllKeys(),
				KeyRange{Key{4}, Key{5}, OpenClosed},
				Key{6}),
			&sppb.KeySet{All: true},
		},
	} {
		gotProto, err := test.ks.keySetProto()
		if err != nil {
			t.Errorf("#%d: %v.proto() returns error %v; want nil error", i, test.ks, err)
		}
		if !reflect.DeepEqual(gotProto, test.wantProto) {
			t.Errorf("#%d: %v.proto() = \n%v\nwant:\n%v", i, test.ks, gotProto.String(), test.wantProto.String())
		}
	}
}
