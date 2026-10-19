# Gemini Enterprise Agent Platform Go SDK

[![Go Reference](https://pkg.go.dev/badge/cloud.google.com/go/agentplatform.svg)](https://pkg.go.dev/cloud.google.com/go/agentplatform)

The Gemini Enterprise Agent Platform SDK for Go enables you to use Google's
state-of-the-art generative AI models (like Gemini) to build AI-powered features
and applications.

For the latest list of available Gemini models on Agent Platform, see the
[Model information](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/google-models)
page in Agent Platform documentation.

## Installation

Add the SDK to your module with `go get cloud.google.com/go/agentplatform`.

## Documentation

You can find complete documentation for the Gemini Enterprise Agent Platform, on
[quickstart](https://docs.cloud.google.com/gemini-enterprise-agent-platform).

For a list of the supported models and their capabilities, see
[model overview](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models)

## Usage Examples

* Samples in the
[package documentation](https://pkg.go.dev/cloud.google.com/go/agentplatform#pkg-examples)

### Create Client:

```go
client, err := agentplatform.NewGenAIClient(ctx, &genai.ClientConfig{
	Project:  project, // If not set, will read from the GOOGLE_CLOUD_PROJECT env var
	Location: location, // If not set, will read from the GOOGLE_CLOUD_LOCATION env var
})
if err != nil {
  panic(err)
}
```

### Create Reasoning Engine:

```go
if _, err := client.AgentEngines.Create(ctx, &types.CreateAgentEngineConfig{}); err != nil {
  panic(err)
}
```

### Wait For Operation

```go
operationName := "projects/PROJEDCT/locations/LOCATION/reasoningEngines/RESOURCE_ID/operations/OPERATION_ID"
for {
  if op, err := client.AgentEngines.GetAgentOperation(ctx, operationName, nil); err != nil {
    panic(err)
  } else if op.Done {
    break
  }
  time.Sleep(5 * time.Second)
}
```

### Get a Reasoning Engine

```go
resourceName := "projects/PROJEDCT/locations/LOCATION/reasoningEngines/RESOURCE_ID"
if _, err := client.AgentEngines.Get(ctx, resourceName, nil); err == nil {
  panic(err)
}
```

### Delete a reasoning engine

```go
resourceName := "projects/PROJEDCT/locations/LOCATION/reasoningEngines/RESOURCE_ID"
if _, err := client.AgentEngines.Delete(ctx, resourceName, nil); err == nil {
  panic(err)
}
```

## Contributing

See [Contributing](https://github.com/googleapis/google-cloud-go/blob/main/CONTRIBUTING.md)
for more information on contributing to the Vertex AI Go SDK.

## License

The contents of this repository are licensed under the
[Apache License, version 2.0](http://www.apache.org/licenses/LICENSE-2.0).

