// Copyright 2023 Google LLC
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

package genai_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/vertexai/genai"

	"google.golang.org/api/iterator"
)

// Your GCP project
const projectID = "your-project"

// A GCP location like "us-central1"; if you're using standard Google-published
// models (like untuned Gemini models), you can keep location blank ("").
const location = "some-gcp-location"

// A model name like "gemini-1.0-pro"
// For custom models from different publishers, prepent the full publisher
// prefix for the model, e.g.:
//
//	modelName = publishers/some-publisher/models/some-model-name
const modelName = "some-model"

func ExampleGenerativeModel_GenerateContent() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)
	model.SetTemperature(0.9)
	resp, err := model.GenerateContent(ctx, genai.Text("What is the average size of a swallow?"))
	if err != nil {
		log.Fatal(err)
	}

	printResponse(resp)
}

// This example shows how to a configure a model. See [GenerationConfig]
// for the complete set of configuration options.
func ExampleGenerativeModel_GenerateContent_config() {
	ctx := context.Background()
	const projectID = "YOUR PROJECT ID"
	const location = "GCP LOCATION"
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.0-pro")
	model.SetTemperature(0.9)
	model.SetTopP(0.5)
	model.SetTopK(20)
	model.SetMaxOutputTokens(100)
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text("You are Yoda from Star Wars.")},
	}
	resp, err := model.GenerateContent(ctx, genai.Text("What is the average size of a swallow?"))
	if err != nil {
		log.Fatal(err)
	}
	printResponse(resp)
}

func ExampleGenerativeModel_GenerateContentStream() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)

	iter := model.GenerateContentStream(ctx, genai.Text("Tell me a story about a lumberjack and his giant ox. Keep it very short."))
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		printResponse(resp)
	}
}

func ExampleGenerativeModel_CountTokens() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)

	resp, err := model.CountTokens(ctx, genai.Text("What kind of fish is this?"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Num tokens:", resp.TotalTokens)
}

func ExampleChatSession() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	model := client.GenerativeModel(modelName)
	cs := model.StartChat()

	send := func(msg string) *genai.GenerateContentResponse {
		fmt.Printf("== Me: %s\n== Model:\n", msg)
		res, err := cs.SendMessage(ctx, genai.Text(msg))
		if err != nil {
			log.Fatal(err)
		}
		return res
	}

	res := send("Can you name some brands of air fryer?")
	printResponse(res)
	iter := cs.SendMessageStream(ctx, genai.Text("Which one of those do you recommend?"))
	for {
		res, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		printResponse(res)
	}

	for i, c := range cs.History {
		log.Printf("    %d: %+v", i, c)
	}
	res = send("Why do you like the Philips?")
	if err != nil {
		log.Fatal(err)
	}
	printResponse(res)
}

func ExampleTool() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	currentWeather := func(city string) string {
		switch city {
		case "New York, NY":
			return "cold"
		case "Miami, FL":
			return "warm"
		default:
			return "unknown"
		}
	}

	// To use functions / tools, we have to first define a schema that describes
	// the function to the model. The schema is similar to OpenAPI 3.0.
	//
	// In this example, we create a single function that provides the model with
	// a weather forecast in a given location.
	schema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"location": {
				Type:        genai.TypeString,
				Description: "The city and state, e.g. San Francisco, CA",
			},
			"unit": {
				Type: genai.TypeString,
				Enum: []string{"celsius", "fahrenheit"},
			},
		},
		Required: []string{"location"},
	}

	weatherTool := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{{
			Name:        "CurrentWeather",
			Description: "Get the current weather in a given location",
			Parameters:  schema,
		}},
	}

	model := client.GenerativeModel("gemini-1.0-pro")

	// Before initiating a conversation, we tell the model which tools it has
	// at its disposal.
	model.Tools = []*genai.Tool{weatherTool}

	// For using tools, the chat mode is useful because it provides the required
	// chat context. A model needs to have tools supplied to it in the chat
	// history so it can use them in subsequent conversations.
	//
	// The flow of message expected here is:
	//
	// 1. We send a question to the model
	// 2. The model recognizes that it needs to use a tool to answer the question,
	//    an returns a FunctionCall response asking to use the CurrentWeather
	//    tool.
	// 3. We send a FunctionResponse message, simulating the return value of
	//    CurrentWeather for the model's query.
	// 4. The model provides its text answer in response to this message.
	session := model.StartChat()

	res, err := session.SendMessage(ctx, genai.Text("What is the weather like in New York?"))
	if err != nil {
		log.Fatal(err)
	}

	part := res.Candidates[0].Content.Parts[0]
	funcall, ok := part.(genai.FunctionCall)
	if !ok {
		log.Fatalf("expected FunctionCall: %v", part)
	}

	if funcall.Name != "CurrentWeather" {
		log.Fatalf("expected CurrentWeather: %v", funcall.Name)
	}

	// Expect the model to pass a proper string "location" argument to the tool.
	locArg, ok := funcall.Args["location"].(string)
	if !ok {
		log.Fatalf("expected string: %v", funcall.Args["location"])
	}

	weatherData := currentWeather(locArg)
	res, err = session.SendMessage(ctx, genai.FunctionResponse{
		Name: weatherTool.FunctionDeclarations[0].Name,
		Response: map[string]any{
			"weather": weatherData,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	printResponse(res)
}

func ExampleGenerativeModel_ToolConfig() {
	// This example shows how to affect how the model uses the tools provided to it.
	// By setting the ToolConfig, you can disable function calling.

	// Assume we have created a Model and have set its Tools field with some functions.
	// See the Example for Tool for details.
	model := &genai.GenerativeModel{}

	// By default, the model will use the functions in its responses if it thinks they are
	// relevant, by returning FunctionCall parts.
	// Here we set the model's ToolConfig to disable function calling completely.
	model.ToolConfig = &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingNone,
		},
	}

	// Subsequent calls to ChatSession.SendMessage will not result in FunctionCall responses.
	session := model.StartChat()
	res, err := session.SendMessage(context.Background(), genai.Text("What is the weather like in New York?"))
	if err != nil {
		log.Fatal(err)
	}
	for _, part := range res.Candidates[0].Content.Parts {
		if _, ok := part.(genai.FunctionCall); ok {
			log.Fatal("did not expect FunctionCall")
		}
	}

	// It is also possible to force a function call by using FunctionCallingAny
	// instead of FunctionCallingNone. See the documentation for FunctionCallingMode
	// for details.
}

func ExampleClient_cachedContent() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	file := genai.FileData{MIMEType: "application/pdf", FileURI: "gs://my-bucket/my-doc.pdf"}
	cc, err := client.CreateCachedContent(ctx, &genai.CachedContent{
		Model:    modelName,
		Contents: []*genai.Content{{Parts: []genai.Part{file}}},
	})
	model := client.GenerativeModelFromCachedContent(cc)
	// Work with the model as usual in this program.
	_ = model

	// Store the CachedContent name for later use.
	if err := os.WriteFile("my-cached-content-name", []byte(cc.Name), 0o644); err != nil {
		log.Fatal(err)
	}

	///////////////////////////////
	// Later, in another process...

	bytes, err := os.ReadFile("my-cached-content-name")
	if err != nil {
		log.Fatal(err)
	}
	ccName := string(bytes)

	// No need to call [Client.GetCachedContent]; the name is sufficient.
	model = client.GenerativeModel(modelName)
	model.CachedContentName = ccName
	// Proceed as usual.
}

func printResponse(resp *genai.GenerateContentResponse) {
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			fmt.Println(part)
		}
	}
	fmt.Println("---")
}
