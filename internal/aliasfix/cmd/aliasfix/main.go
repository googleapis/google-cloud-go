// Copyright 2022 Google LLC
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

package main

import (
	"flag"
	"log"

	"cloud.google.com/go/internal/aliasfix"
)

func main() {
	flag.Parse()
	path := flag.Arg(0)
	if path == "" {
		log.Fatalf("expected one argument -- path to the directory needing updates")
	}
	if err := aliasfix.ProcessPath(path); err != nil {
		log.Fatal(err)
	}
}
