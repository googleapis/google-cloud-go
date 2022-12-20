// Copyright 2022 Google LLC
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

package internal

import (
	"google.golang.org/api/option"
)

// StorageConfig contains the Storage client option configuration that can be
// set through StorageOptions. It is defined in the internal package so that
// both the storageoption and storage packages can access it.
type StorageConfig struct {
	UseJSONforReads bool
	ReadAPIWasSet   bool
}

// A StorageOption is an option for a Google Storage client.
type StorageOption interface {
	option.ClientOption
	ApplyOpt(*StorageConfig)
}
