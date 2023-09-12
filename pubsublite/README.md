## Pub/Sub Lite [![Go Reference](https://pkg.go.dev/badge/cloud.google.com/go/pubsublite.svg)](https://pkg.go.dev/cloud.google.com/go/pubsublite)

- [About Pub/Sub Lite](https://cloud.google.com/pubsub/lite)
- [Client library documentation](https://cloud.google.com/pubsub/lite/docs/reference/libraries)
- [API documentation](https://cloud.google.com/pubsub/lite/docs/apis)
- [Go client documentation](https://pkg.go.dev/cloud.google.com/go/pubsublite)
- [Complete sample programs](https://github.com/GoogleCloudPlatform/golang-samples/tree/main/pubsublite)


### Example Usage

[snip]:# (imports)
```go
import (
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
)
```

To publish messages to a topic:

[snip]:# (publish)
```go
// Create a PublisherClient for topic1 in zone us-central1-b.
// See https://cloud.google.com/pubsub/lite/docs/locations for available regions
// and zones.
const topic = "projects/project-id/locations/us-central1-b/topics/topic1"
publisher, err := pscompat.NewPublisherClient(ctx, topic)
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
// Create a SubscriberClient for subscription1 in zone us-central1-b.
const subscription = "projects/project-id/locations/us-central1-b/subscriptions/subscription1"
subscriber, err := pscompat.NewSubscriberClient(ctx, subscription)
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
