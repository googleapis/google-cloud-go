// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*

Package gcloudtest provides a set of primitive tools to allow you to
write unit tests with gcloud-golang library. A typical use is
recording HTTP requests that gcloud-golang library issues, and
providing mocked HTTP responses for mocking remote API endpoints'
behavior. However, to create mocked responses, you need a deep
knowledge about the raw protocol used by the remote API that you're
using. Therefore, MockTransport deferes this work to another
RoundTripper that test writers need to provide via the Register
function.

Although this package will help developers write unit tests much
easier, you may still need to match the URL and the HTTP method of the
HTTP requests against the ones of the actual API endpoints, and you
also need to hand craft mocked HTTP responses that emurate the API's
behavior.

Hopefully in the near future, we add some higher level testing
libraries for eash sub packages that understands the raw API behavior
so that you can write unit tests only with higher level knowledge of
the API.

Example unit test

Let's say you're using Cloud Pub/Sub and have the following code.

    type Acknowledger struct {
	    ctx         context.Context
        blacklist []string
    }

    func (a *Acknowledger) Ack(sub string, ackID string) error {
        for _, v := range a.blacklist {
            if sub == v {
                return fmt.Errorf("Now the sub '%s' is forbidden", sub)
            }
        }
        return pubsub.Ack(a.ctx, sub, ackID)
    }

Then first you can create a simple RoundTripper as follows.

    type intRoundTripper int

    func (i intRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
        return &http.Response{
            Status:     fmt.Sprintf("%d OK", int(i)),
            StatusCode: int(i),
            Body:       ioutil.NopCloser(bytes.NewBufferString("OK")),
        }, nil
    }

You can use MockTransport and intRoundTripper as follows.


Then your test function.

    func TestAck(t *testing.T) {
        tests := []struct {
            sub      string
            ackID    string
            shouldOK bool
        }{
            {"sub1", "ack1", true},
            {"sub2", "ack2", true},
            {"sub3", "ack3", false},
        }
        blacklist := []string{"sub3"}
        numOK := 2

        mock := gcloudtest.NewMockTransport()
        mock.Register(intRoundTripper(200))
        ctx := cloud.NewContext("project-id", &http.Client{Transport: mock})
        a := &Acknowledger{ctx: ctx, blacklist: blacklist}

        for _, test := range tests {
            req, err := a.Ack(test.sub, test.ackID)
            if err != nil && test.shouldOK {
                t.Errorf("The test shouldn't fail, but it failed, %v", test)
            }
        }
        if mock.Len() != numOK {
            t.Errorf("There should be exact %d API calls, but %d", numOK, mock.Len())
        }
        // Optionally vet the issued requests, but it needs knowledge
        // about raw API protocol.
        for mock.Len() > 0 {
            req := mock.GetRequest(0)
            // vet the req
        }
    }

Higher level libraries

As you can see, MockTransport just automates the recording part, so
you still need to know the raw API protocol in order to 1) switch your
mocked responses according to the URL, the HTTP method and the body of
the requests, 2) vet the recorded requests.

Ideally gcloud-golang has higher level libraries in each sub packages
to make the unit tests much easier.

*/
package gcloudtest
