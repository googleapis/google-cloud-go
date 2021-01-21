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

package testutil_test

import (
	"testing"

	"cloud.google.com/go/pubsub/internal/testutil"
)

func TestVerifyKeyOrdering(t *testing.T) {
	for _, tc := range []struct {
		name          string
		publishedMsgs []testutil.OrderedKeyMsg
		receivedMsgs  []testutil.OrderedKeyMsg
		wantErr       bool
	}{
		{
			name: "correct despite different ordering",
			publishedMsgs: []testutil.OrderedKeyMsg{
				{Key: "some-key-1", Data: "some-datum-1"},
				{Key: "some-key-2", Data: "some-datum-1"},
				{Key: "some-key-1", Data: "some-datum-2"},
				{Key: "some-key-3", Data: "some-datum-1"},
				{Key: "some-key-1", Data: "some-datum-3"},
				{Key: "some-key-4", Data: "some-datum-1"},
			},
			receivedMsgs: []testutil.OrderedKeyMsg{
				{Key: "some-key-2", Data: "some-datum-1"},
				{Key: "some-key-1", Data: "some-datum-1"},
				{Key: "some-key-3", Data: "some-datum-1"},
				{Key: "some-key-4", Data: "some-datum-1"},
				{Key: "some-key-1", Data: "some-datum-2"},
				{Key: "some-key-1", Data: "some-datum-3"},
			},
			wantErr: false,
		},
		{
			name: "received something we didnt publish",
			publishedMsgs: []testutil.OrderedKeyMsg{
				{Key: "some-key-1", Data: "some-datum-1"},
			},
			receivedMsgs: []testutil.OrderedKeyMsg{
				{Key: "some-key-1", Data: "some-datum-2"},
			},
			wantErr: true,
		},
		{
			name: "published message missing",
			publishedMsgs: []testutil.OrderedKeyMsg{
				{Key: "some-key-1", Data: "some-datum-1"},
			},
			receivedMsgs: []testutil.OrderedKeyMsg{},
			wantErr:      true,
		},
		// TODO(deklerk): account for consistent redelivery.
		//{
		//	name: "correct despite consistent redlivery",
		//	publishedMsgs: []testutil.OrderedKeyMsg{
		//		{Key: "some-key-1", Data: "some-datum-1"},
		//		{Key: "some-key-1", Data: "some-datum-2"},
		//		{Key: "some-key-1", Data: "some-datum-3"},
		//		{Key: "some-key-1", Data: "some-datum-4"},
		//		{Key: "some-key-1", Data: "some-datum-5"},
		//	},
		//	// Messages 2 and 3 are redelivered twice.
		//	receivedMsgs: []testutil.OrderedKeyMsg{
		//		{Key: "some-key-1", Data: "some-datum-1"},
		//		{Key: "some-key-1", Data: "some-datum-2"},
		//		{Key: "some-key-1", Data: "some-datum-3"},
		//		{Key: "some-key-1", Data: "some-datum-2"},
		//		{Key: "some-key-1", Data: "some-datum-3"},
		//		{Key: "some-key-1", Data: "some-datum-2"},
		//		{Key: "some-key-1", Data: "some-datum-3"},
		//		{Key: "some-key-1", Data: "some-datum-4"},
		//		{Key: "some-key-1", Data: "some-datum-5"},
		//	},
		//	wantErr:      true,
		//},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := testutil.VerifyKeyOrdering(tc.publishedMsgs, tc.receivedMsgs)
			if tc.wantErr {
				if err == nil {
					t.Fatal("wanted err, but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("wanted nil, got err:\n\t%v", err)
				}
			}
		})
	}
}
