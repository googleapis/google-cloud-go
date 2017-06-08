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
write unit tests with gcloud-golang library.

Example unit test

Let's say you're using Cloud Pub/Sub and have the following code.

    // Acknowledger just call Ack with a knowledge of the blacklist
    type Acknowledger struct {
	    ctx         context.Context
        // for some reason, you want to have a blacklist for
        // subscription to supress acknowledgement.
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

In most case, you start writing tests from implementing your own RoundTripper

    type assertTpt struct {
        exp    *http.Request
        resp *http.Response
        t         *testing.T
    }

    func (tr *assertTpt) RoundTrip(r *http.Request) (*http.Response, error) {
        if tr.exp == nil {
            tr.t.Errorf("There should not be any requests.")
        }
        if tr.exp.Method != r.Method {
            tr.t.Errorf("Method should be %s, but got %s.",
                tr.exp.Method, r.Method)
        }
        if tr.exp.URL.String() != r.URL.String() {
            tr.t.Errorf("URL should be %s, but got %s.", tr.exp.URL, r.URL)
        }
        bodyExp, err := ioutil.ReadAll(tr.exp.Body)
        if err != nil {
            tr.t.Errorf("ReadAll failed for expected body, %v", err)
        }
        body, err := ioutil.ReadAll(r.Body)
        if err != nil {
            tr.t.Errorf("ReadAll failed, %v", err)
        }
        if string(bodyExp) != string(body) {
            tr.t.Errorf("Body should be %s, but got %s", bodyExp, body)
        }
        return resp, nil
    }

Then your test function.

    func TestAck(t *testing.T) {
        tests := []struct {
            sub         string
            ackID       string
            blacklisted bool
        }{
            {"sub1", "ack1", false},
            {"sub2", "ack2", false},
            {"sub3", "ack3", true},
        }
        blacklist := []string{"sub3"}
        resp200 := &http.Response{
            Status:     "200 OK",
            StatusCode: 200,
            Body:       ioutil.NopCloser(bytes.NewBufferString("")),
        }
        pid := "project-id"

        ctx := cloud.NewContext(pid, &http.Client{})

        for _, test := range tests {
            expectReq, err := pubsubtest.AckRequest(ctx, test.sub, test.ackID)
            if err != nil {
                t.Errorf("AckRequests failed, %v", err)
            }
            var mock *AssertTpt
            if test.blacklisted {
                mock = &assertTpt{exp: nil, resp: resp200, t: t}
            } else {
                mock := &assertTpt{exp: expectReq, resp: resp200, t: t}
            }
            mockCtx := cloud.NewContext(pid, &http.Client{Transport: mock})
            a := &Acknowledger{ctx: mockCtx, blacklist: blacklist}
            req, err := a.Ack(test.sub, test.ackID)
            if err == nil && test.blacklissted {
                t.Errorf("Ack should fail, but it succeeded, %v", test)
            }
            if err != nil && !test.blacklisted {
                t.Errorf("Ack shouldn't fail, but it failed, %v", test)
            }
        }
    }

*/
package gcloudtest
