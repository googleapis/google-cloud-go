// Copyright 2020 Google LLC
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

package ps

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/wire"
)

func TestPublishSettingsToWireSettings(t *testing.T) {
	for _, tc := range []struct {
		desc         string
		settings     PublishSettings
		wantSettings wire.PublishSettings
	}{
		{
			desc:     "default settings",
			settings: DefaultPublishSettings,
			wantSettings: wire.PublishSettings{
				DelayThreshold:    DefaultPublishSettings.DelayThreshold,
				CountThreshold:    DefaultPublishSettings.CountThreshold,
				ByteThreshold:     DefaultPublishSettings.ByteThreshold,
				Timeout:           DefaultPublishSettings.Timeout,
				BufferedByteLimit: DefaultPublishSettings.BufferedByteLimit,
				ConfigPollPeriod:  wire.DefaultPublishSettings.ConfigPollPeriod,
				Framework:         wire.FrameworkCloudPubSubShim,
			},
		},
		{
			desc:         "zero settings",
			settings:     PublishSettings{},
			wantSettings: DefaultPublishSettings.toWireSettings(),
		},
		{
			desc: "positive values",
			settings: PublishSettings{
				DelayThreshold:    2,
				CountThreshold:    3,
				ByteThreshold:     4,
				Timeout:           5,
				BufferedByteLimit: 6,
			},
			wantSettings: wire.PublishSettings{
				DelayThreshold:    2,
				CountThreshold:    3,
				ByteThreshold:     4,
				Timeout:           5,
				BufferedByteLimit: 6,
				ConfigPollPeriod:  wire.DefaultPublishSettings.ConfigPollPeriod,
				Framework:         wire.FrameworkCloudPubSubShim,
			},
		},
		{
			desc: "negative values",
			settings: PublishSettings{
				DelayThreshold:    -2,
				CountThreshold:    -3,
				ByteThreshold:     -4,
				Timeout:           -5,
				BufferedByteLimit: -6,
			},
			wantSettings: wire.PublishSettings{
				DelayThreshold:    -2,
				CountThreshold:    -3,
				ByteThreshold:     -4,
				Timeout:           -5,
				BufferedByteLimit: -6,
				ConfigPollPeriod:  wire.DefaultPublishSettings.ConfigPollPeriod,
				Framework:         wire.FrameworkCloudPubSubShim,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := testutil.Diff(tc.settings.toWireSettings(), tc.wantSettings); diff != "" {
				t.Errorf("PublishSettings.toWireSettings() got: -, want: +\n%s", diff)
			}
		})
	}
}

func TestReceiveSettingsToWireSettings(t *testing.T) {
	for _, tc := range []struct {
		desc         string
		settings     ReceiveSettings
		wantSettings wire.ReceiveSettings
	}{
		{
			desc:     "default settings",
			settings: DefaultReceiveSettings,
			wantSettings: wire.ReceiveSettings{
				MaxOutstandingMessages: DefaultReceiveSettings.MaxOutstandingMessages,
				MaxOutstandingBytes:    DefaultReceiveSettings.MaxOutstandingBytes,
				Timeout:                DefaultReceiveSettings.Timeout,
				Framework:              wire.FrameworkCloudPubSubShim,
			},
		},
		{
			desc:         "zero settings",
			settings:     ReceiveSettings{},
			wantSettings: DefaultReceiveSettings.toWireSettings(),
		},
		{
			desc: "positive values",
			settings: ReceiveSettings{
				MaxOutstandingMessages: 2,
				MaxOutstandingBytes:    3,
				Timeout:                4,
				Partitions:             []int{5, 6},
			},
			wantSettings: wire.ReceiveSettings{
				MaxOutstandingMessages: 2,
				MaxOutstandingBytes:    3,
				Timeout:                4,
				Partitions:             []int{5, 6},
				Framework:              wire.FrameworkCloudPubSubShim,
			},
		},
		{
			desc: "negative values",
			settings: ReceiveSettings{
				MaxOutstandingMessages: -2,
				MaxOutstandingBytes:    -3,
				Timeout:                -4,
				Partitions:             []int{-5, -6},
			},
			wantSettings: wire.ReceiveSettings{
				MaxOutstandingMessages: -2,
				MaxOutstandingBytes:    -3,
				Timeout:                -4,
				Partitions:             []int{-5, -6},
				Framework:              wire.FrameworkCloudPubSubShim,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := testutil.Diff(tc.settings.toWireSettings(), tc.wantSettings); diff != "" {
				t.Errorf("ReceiveSettings.toWireSettings() got: -, want: +\n%s", diff)
			}
		})
	}
}
