## Cloud Pub/Sub Lite [![GoDoc](https://godoc.org/cloud.google.com/go/pubsublite?status.svg)](https://pkg.go.dev/cloud.google.com/go/pubsublite)

- [About Cloud Pub/Sub Lite](https://cloud.google.com/pubsub/lite)
- [Client library documentation](https://cloud.google.com/pubsub/lite/docs/reference/libraries)
- [API documentation](https://cloud.google.com/pubsub/lite/docs/apis)
- [Go client documentation](https://pkg.go.dev/cloud.google.com/go/pubsublite)

*This library is in ALPHA. Backwards-incompatible changes may be made before
 stable v1.0.0 is released.*

### Example Usage

[snip]:# (imports)
```go
import (
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite"
	"cloud.google.com/go/pubsublite/ps"
)
```

To publish messages to a topic:

[snip]:# (publish)
```go
// Create a PublisherClient for topic1.
// See https://cloud.google.com/pubsub/lite/docs/locations for available zones.
topic := pubsublite.TopicPath{
    Project: "project-id",
    Zone:    "us-central1-b",
    TopicID: "topic1",
}
publisher, err := ps.NewPublisherClient(ctx, ps.DefaultPublishSettings, topic)
if err != nil {
    log.Fatal(err)
}

// Publish "hello world".
res := publisher.Publish(ctx, &pubsub.Message{
	Data: []byte("hello world"),
})
// The publish happens asynchronously.
// Later, you can get the result from res:
...
msgID, err := res.Get(ctx)
if err != nil {
	log.Fatal(err)
}
```

To receive messages for a subscription:

[snip]:# (subscribe)
```go
// Create a SubscriberClient for subscription1.
subscription := pubsublite.SubscriptionPath{
    Project:        "project-id",
    Zone:           "us-central1-b",
    SubscriptionID: "subscription1",
}
subscriber, err := ps.NewSubscriberClient(ctx, ps.DefaultReceiveSettings, subscription)
if err != nil {
	log.Fatal(err)
}

// Use a callback to receive messages.
// Call cancel() to stop receiving messages.
cctx, cancel := context.WithCancel(ctx)
err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
	fmt.Println(m.Data)
	m.Ack() // Acknowledge that we've consumed the message.
})
if err != nil {
	log.Println(err)
}
```
