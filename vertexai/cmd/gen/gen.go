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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/vertexai/genai"

	"google.golang.org/api/iterator"
)

var (
	project   = flag.String("project", "", "project ID")
	location  = flag.String("location", "", "location")
	temp      = flag.Float64("t", 0.2, "temperature")
	model     = flag.String("model", "", "model")
	streaming = flag.Bool("stream", false, "using the streaming API")
)

func main() {
	flag.Parse()
	if *project == "" || *location == "" || *model == "" {
		log.Fatal("need -project, -location, and -model")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, *project, *location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	model := client.GenerativeModel(*model)
	model.Temperature = float32(*temp)

	model.SafetySettings = []*genai.SafetySetting{
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockLowAndAbove,
		},
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockLowAndAbove,
		},
	}

	text := strings.Join(flag.Args(), " ")
	if *streaming {
		iter := model.GenerateContentStream(ctx, genai.Text(text))
		for {
			res, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				showError(err)
			}
			showJSON(res)
			fmt.Println("---")
		}
	} else {
		res, err := model.GenerateContent(ctx, genai.Text(text))
		if err != nil {
			showError(err)
		}
		showJSON(res)
	}
}

func showJSON(x any) {
	bs, err := json.MarshalIndent(x, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", bs)
}

func showError(err error) {
	var berr *genai.BlockedError
	if errors.As(err, &berr) {
		fmt.Println("ERROR:")
		showJSON(err)
	} else {
		fmt.Printf("ERROR: %s\n", err)
	}
	os.Exit(1)
}
