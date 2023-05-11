// Copyright 2021 Google LLC
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

package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	vkit "cloud.google.com/go/pubsub/apiv1"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestClient_CustomRetry(t *testing.T) {
	ctx := context.Background()
	pco := &vkit.PublisherCallOptions{
		Publish: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.Unavailable, codes.DeadlineExceeded,
				}, gax.Backoff{
					Initial:    200 * time.Millisecond,
					Max:        30000 * time.Millisecond,
					Multiplier: 1.25,
				})
			}),
		},
	}
	sco := &vkit.SubscriberCallOptions{}
	c, err := NewClientWithConfig(ctx, "some-project", &ClientConfig{PublisherCallOptions: pco, SubscriberCallOptions: sco})
	if err != nil {
		t.Fatalf("failed to create client with config: %v", err)
	}

	cs := &gax.CallSettings{}
	// This is the default retry setting.
	c.pubc.CallOptions.Publish[0].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{100000000 60000000000 1.3 0} [10 1 13 8 2 14 4]}"; got != want {
		t.Fatalf("got: %v, want: %v", got, want)
	}

	// This is the custom retry setting.
	c.pubc.CallOptions.Publish[1].Resolve(cs)
	if got, want := fmt.Sprintf("%v", cs.Retry()), "&{{200000000 30000000000 1.25 0} [14 4]}"; got != want {
		t.Fatalf("merged CallOptions is incorrect: got %v, want %v", got, want)
	}
}

func TestClient_ApplyClientConfig(t *testing.T) {
	ctx := context.Background()
	srv := pstest.NewServer()
	// Add a retry for an obscure error.
	pco := &vkit.PublisherCallOptions{
		Publish: []gax.CallOption{
			gax.WithRetry(func() gax.Retryer {
				return gax.OnCodes([]codes.Code{
					codes.DataLoss,
				}, gax.Backoff{
					Initial:    200 * time.Millisecond,
					Max:        30000 * time.Millisecond,
					Multiplier: 1.25,
				})
			}),
		},
	}
	c, err := NewClientWithConfig(ctx, "P", &ClientConfig{
		PublisherCallOptions: pco,
	},
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()))
	if err != nil {
		t.Fatal(err)
	}

	srv.SetAutoPublishResponse(false)
	// Create a fake publish response with the obscure error we are retrying.
	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{},
	}, status.Errorf(codes.DataLoss, "obscure error"))

	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{"1"},
	}, nil)

	topic, err := c.CreateTopic(ctx, "t")
	if err != nil {
		t.Fatal(err)
	}
	res := topic.Publish(ctx, &Message{
		Data: []byte("test"),
	})
	if id, err := res.Get(ctx); err != nil {
		t.Fatalf("got error from res.Get(): %v", err)
	} else {
		if id != "1" {
			t.Fatalf("got wrong message id from server, got %s, want 1", id)
		}
	}
}
