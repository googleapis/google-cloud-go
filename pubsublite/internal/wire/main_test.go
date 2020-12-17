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
	"os"
	"testing"

	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/api/option"
)

var (
	// Initialized in TestMain.
	mockServer     test.MockServer
	testClientOpts []option.ClientOption
)

func TestMain(m *testing.M) {
	flag.Parse()

	testServer, clientOpts := test.NewServerWithConn()
	mockServer = testServer.LiteServer
	testClientOpts = clientOpts

	exit := m.Run()
	testServer.Close()
	os.Exit(exit)
}
