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

package adapt

import "cloud.google.com/go/internal/optional"

// Option to customize proto descriptor conversion.
type Option interface {
	applyCustomClientOpt(*customConfig)
}

// type for collecting custom adapt Option values.
type customConfig struct {
	useTimestampWellKnownType bool
	useProto3                 bool
}

type customOption struct {
	useTimestampWellKnownType optional.Bool
}

// WithTimestampWellKnownType defines that table fields of type Timestamp, are mapped
// as Google's WKT timestamppb.Timestamp.
func WithTimestampWellKnownType(useTimestampWellKnownType bool) Option {
	return &customOption{useTimestampWellKnownType: useTimestampWellKnownType}
}

func (o *customOption) applyCustomClientOpt(cfg *customConfig) {
	if o.useTimestampWellKnownType != nil {
		cfg.useTimestampWellKnownType = optional.ToBool(o.useTimestampWellKnownType)
	}
}
