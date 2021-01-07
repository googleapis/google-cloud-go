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

// +build go1.12

package wire

import (
	"runtime/debug"
	"strconv"
	"strings"
)

const pubsubLiteModulePath = "cloud.google.com/go/pubsublite"

type version struct {
	Major string
	Minor string
}

// libraryVersion attempts to determine the pubsublite module version.
func libraryVersion() (version, bool) {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		return pubsubliteModuleVersion(buildInfo)
	}
	return version{}, false
}

// pubsubliteModuleVersion extracts the module version from BuildInfo embedded
// in the binary. Only applies to binaries built with module support.
func pubsubliteModuleVersion(buildInfo *debug.BuildInfo) (version, bool) {
	for _, dep := range buildInfo.Deps {
		if dep.Path == pubsubLiteModulePath {
			return parseModuleVersion(dep.Version)
		}
	}
	return version{}, false
}

func parseModuleVersion(value string) (v version, ok bool) {
	if strings.HasPrefix(value, "v") {
		value = value[1:]
	}
	components := strings.Split(value, ".")
	if len(components) >= 2 {
		if _, err := strconv.ParseInt(components[0], 10, 32); err != nil {
			return
		}
		if _, err := strconv.ParseInt(components[1], 10, 32); err != nil {
			return
		}
		v = version{Major: components[0], Minor: components[1]}
		ok = true
	}
	return
}
