# Vertex AI Go SDK

[![Go Reference](https://pkg.go.dev/badge/cloud.google.com/go/vertexai.svg)](https://pkg.go.dev/cloud.google.com/go/vertexai)

The Vertex AI Go SDK enables developers to use Google's state-of-the-art 
generative AI models (like Gemini) to build AI-powered features and applications.
This SDK supports use cases like:
- Generate text from text-only input
- Generate text from text-and-images input (multimodal)
- Build multi-turn conversations (chat)

For example, with just a few lines of code, you can access Gemini's multimodal
capabilities to generate text from text-and-image input.

```go
model := client.GenerativeModel("gemini-pro-vision")
img := genai.ImageData("jpeg", image_bytes)
prompt := genai.Text("Please give me a recipe for this:")
resp, err := model.GenerateContent(ctx, img, prompt)
```

## Installation and usage

Add the SDK to your module with `go get cloud.google.com/go/vertexai/genai`.

For detailed instructions, you can find a [quickstart](http://cloud.google.com/vertex-ai/docs/generative-ai/start/quickstarts/quickstart-multimodal)
for the Vertex AI Go SDK in the Google Cloud documentation.

## Documentation

You can find complete documentation for the Vertex AI SDKs and the Gemini
model in the Google Cloud documentation: https://cloud.google.com/vertex-ai/docs/generative-ai/learn/overview

For a list of the supported models and their capabilities, see https://cloud.google.com/vertex-ai/docs/generative-ai/learn/model-versioning

You can also find information about this SDK in the
[Go package documentation](https://pkg.go.dev/cloud.google.com/go/vertexai).

Check out some usage samples:

* In the [package documentation](https://pkg.go.dev/cloud.google.com/go/vertexai/genai#pkg-examples)
* In the [vertexai golang-samples repository](https://github.com/GoogleCloudPlatform/golang-samples/tree/main/vertexai)

## Contributing

See [Contributing](https://github.com/googleapis/google-cloud-go/blob/main/CONTRIBUTING.md)
for more information on contributing to the Vertex AI Go SDK.

## License

The contents of this repository are licensed under the
[Apache License, version 2.0](http://www.apache.org/licenses/LICENSE-2.0).


