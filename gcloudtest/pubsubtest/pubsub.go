// Copyright 2014 Google Inc. All Rights Reserved.
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

// package gcloudtest is a core part of the gcloud-golang testing tool.
package pubsubtest

import (
	"errors"
	"net/http"

	"code.google.com/p/go.net/context"
)

// AckRequest invokes pubsub.Ack with given parameters and recording
// RoundTripper then returns the recorded request.
func AckRequest(
	ctx context.Context, sub string, ackID ...string,
) (*http.Request, error) {
	pid := ctx.Value("vals").(map[string]interface{})["project_id"].(string)
	recorder := &SimpleRecorder{}
	recCtx := cloud.NewContext(pid, &http.Client{Transport: recorder})
	err := pubsub.Ack(recCtx, sub, ackID...)
	if err != nil {
		return nil, fmt.ErrorF("pubsub.Ack failed, %v", err)
	}
	reqs := recorder.GetRequests()
	if reqs == nil || len(reqs) == 0 {
		return nil, errors.New("No request recorded.")
	}
	return reqs[0], nil
}
