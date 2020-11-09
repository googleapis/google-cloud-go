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

package wire

import (
	"testing"
	"time"
)

func TestValidatePublishSettings(t *testing.T) {
	for _, tc := range []struct {
		desc string
		// settingsFunc is passed DefaultPublishSettings
		settingsFunc func(settings PublishSettings) PublishSettings
		wantErr      bool
	}{
		{
			desc: "valid: default",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				return DefaultPublishSettings
			},
			wantErr: false,
		},
		{
			desc: "valid: max",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				return PublishSettings{
					CountThreshold: MaxPublishRequestCount,
					ByteThreshold:  MaxPublishRequestBytes,
					// These have no max bounds, check large values.
					DelayThreshold:    24 * time.Hour,
					Timeout:           24 * time.Hour,
					BufferedByteLimit: 1e10,
				}
			},
			wantErr: false,
		},
		{
			desc: "invalid: zero CountThreshold",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.CountThreshold = 0
				return settings
			},
			wantErr: true,
		},
		{
			desc: "invalid: CountThreshold over MaxPublishRequestCount",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.CountThreshold = MaxPublishRequestCount + 1
				return settings
			},
			wantErr: true,
		},
		{
			desc: "invalid: ByteThreshold over MaxPublishRequestBytes",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.ByteThreshold = MaxPublishRequestBytes + 1
				return settings
			},
			wantErr: true,
		},
		{
			desc: "invalid: zero ByteThreshold",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.ByteThreshold = 0
				return settings
			},
			wantErr: true,
		},
		{
			desc: "invalid: zero DelayThreshold",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.DelayThreshold = time.Duration(0)
				return settings
			},
			wantErr: true,
		},
		{
			desc: "invalid: zero Timeout",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.Timeout = time.Duration(0)
				return settings
			},
			wantErr: true,
		},
		{
			desc: "invalid: zero BufferedByteLimit",
			settingsFunc: func(settings PublishSettings) PublishSettings {
				settings.BufferedByteLimit = 0
				return settings
			},
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			settings := tc.settingsFunc(DefaultPublishSettings)
			gotErr := validatePublishSettings(settings)
			if (gotErr != nil) != tc.wantErr {
				t.Errorf("validatePublishSettings(%v) = %v, want err=%v", settings, gotErr, tc.wantErr)
			}
		})
	}
}
