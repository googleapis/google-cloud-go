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
	"flag"
	"log"
	"os"
	"testing"

	"cloud.google.com/go/pubsublite/internal/test"
)

var (
	// Initialized in TestMain.
	testServer *test.Server
	mockServer test.MockServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	var err error
	if testServer, err = test.NewServer(); err != nil {
		log.Fatal(err)
	}
	mockServer = testServer.LiteServer

	exit := m.Run()
	testServer.Close()
	os.Exit(exit)
}
