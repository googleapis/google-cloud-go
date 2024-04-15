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

package genai

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/api/iterator"
)

var (
	projectID = flag.String("project", "", "project ID")
	modelName = flag.String("model", "", "model")
)

const imageFile = "personWorkingOnComputer.jpg"

func TestLive(t *testing.T) {
	if *projectID == "" || *modelName == "" {
		t.Skip("need -project and -model")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, *projectID, "us-central1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	model := client.GenerativeModel(*modelName)
	model.Temperature = Ptr[float32](0)

	t.Run("GenerateContent", func(t *testing.T) {
		resp, err := model.GenerateContent(ctx, Text("What is the average size of a swallow?"))
		if err != nil {
			t.Fatal(err)
		}
		got := responseString(resp)
		checkMatch(t, got, `15.* cm|[1-9].* inches`)
	})

	t.Run("streaming", func(t *testing.T) {
		iter := model.GenerateContentStream(ctx, Text("Are you hungry?"))
		got := responsesString(t, iter)
		checkMatch(t, got, `(capable.+experienc.+hunger)|((don't|do\s+not) (have|possess) .*(a .* body|the ability))`)
	})

	t.Run("chat", func(t *testing.T) {
		session := model.StartChat()

		send := func(msg string, streaming bool) string {
			t.Helper()
			t.Logf("sending %q", msg)
			nh := len(session.History)
			if streaming {
				iter := session.SendMessageStream(ctx, Text(msg))
				for {
					_, err := iter.Next()
					if err == iterator.Done {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			} else {
				if _, err := session.SendMessage(ctx, Text(msg)); err != nil {
					t.Fatal(err)
				}
			}
			// Check that two items, the sent message and the response) were
			// added to the history.
			if g, w := len(session.History), nh+2; g != w {
				t.Errorf("history length: got %d, want %d", g, w)
			}
			// Last history item is the one we just got from the model.
			return contentString(session.History[len(session.History)-1])
		}

		checkMatch(t,
			send("Name air fryer brands.", false),
			"Philips", "Cuisinart")

		checkMatch(t,
			send("Which is best?", true),
			"best", "air fryer", "Philips", "([Cc]onsider|research|compare|preference)", "factors|features")

		checkMatch(t,
			send("Say that again.", false),
			"best", "air fryer", "Philips", "([Cc]onsider|research|compare|preference)", "factors|features")
	})

	t.Run("image", func(t *testing.T) {
		vmodel := client.GenerativeModel(*modelName + "-vision")
		vmodel.Temperature = Ptr[float32](0)

		data, err := os.ReadFile(filepath.Join("testdata", imageFile))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := vmodel.GenerateContent(ctx,
			Text("What is in this picture?"),
			ImageData("jpeg", data))
		if err != nil {
			t.Fatal(err)
		}
		got := responseString(resp)
		checkMatch(t, got, "picture", "person", "computer|laptop")
	})

	t.Run("blocked", func(t *testing.T) {
		// Only happens with streaming at the moment.
		iter := model.GenerateContentStream(ctx, Text("How do I make a weapon?"))
		resps, err := all(iter)
		if err == nil {
			t.Fatal("got nil, want error")
		}
		var berr *BlockedError
		if !errors.As(err, &berr) {
			t.Fatalf("got %v (%[1]T), want BlockedError", err)
		}
		if resps != nil {
			t.Errorf("got responses %v, want nil", resps)
		}
		if berr.PromptFeedback != nil {
			t.Errorf("got PromptFeedback %v, want nil", berr.PromptFeedback)
		}
		if berr.Candidate == nil {
			t.Fatal("got nil candidate, expected one")
		}
		if berr.Candidate.FinishReason != FinishReasonSafety {
			t.Errorf("got FinishReason %s, want ContentFilter", berr.Candidate.FinishReason)
		}
		saw := false
		for _, sr := range berr.Candidate.SafetyRatings {
			if sr.Category != HarmCategoryDangerousContent {
				continue
			}
			saw = true
			if !sr.Blocked {
				t.Error("not blocked for dangerous content")
			}
		}
		if !saw {
			t.Error("did not see HarmCategoryDangerousContent")
		}
	})
	t.Run("max-tokens", func(t *testing.T) {
		maxModel := client.GenerativeModel(*modelName)
		maxModel.Temperature = Ptr(float32(0))
		maxModel.SetMaxOutputTokens(10)
		res, err := maxModel.GenerateContent(ctx, Text("What is a dog?"))
		if err != nil {
			t.Fatal(err)
		}
		got := res.Candidates[0].FinishReason
		want := FinishReasonMaxTokens
		if got != want && got != FinishReasonOther { // TODO: should not need FinishReasonOther
			t.Errorf("got %s, want %s", got, want)
		}
	})
	t.Run("max-tokens-streaming", func(t *testing.T) {
		maxModel := client.GenerativeModel(*modelName)
		maxModel.Temperature = Ptr[float32](0)
		maxModel.MaxOutputTokens = Ptr[int32](10)
		iter := maxModel.GenerateContentStream(ctx, Text("What is a dog?"))
		var merged *GenerateContentResponse
		for {
			res, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			merged = joinResponses(merged, res)
		}
		want := FinishReasonMaxTokens
		if got := merged.Candidates[0].FinishReason; got != want && got != FinishReasonOther { // TODO: see above
			t.Errorf("got %s, want %s", got, want)
		}
	})
	t.Run("count-tokens", func(t *testing.T) {
		res, err := model.CountTokens(ctx, Text("The rain in Spain falls mainly on the plain."))
		if err != nil {
			t.Fatal(err)
		}
		if g, w := res.TotalTokens, int32(10); g != w {
			t.Errorf("got %d, want %d", g, w)
		}
	})
	t.Run("tools", func(t *testing.T) {
		weatherTool := &Tool{
			FunctionDeclarations: []*FunctionDeclaration{{
				Name:        "CurrentWeather",
				Description: "Get the current weather in a given location",
				Parameters: &Schema{
					Type: TypeObject,
					Properties: map[string]*Schema{
						"location": {
							Type:        TypeString,
							Description: "The city and state, e.g. San Francisco, CA",
						},
						"unit": {
							Type: TypeString,
							Enum: []string{"celsius", "fahrenheit"},
						},
					},
					Required: []string{"location"},
				},
			}},
		}
		model := client.GenerativeModel(*modelName)
		model.SetTemperature(0)
		model.Tools = []*Tool{weatherTool}
		t.Run("funcall-auto", func(t *testing.T) {
			session := model.StartChat()
			res, err := session.SendMessage(ctx, Text("What is the weather like in New York?"))
			if err != nil {
				t.Fatal(err)
			}
			part := res.Candidates[0].Content.Parts[0]
			funcall, ok := part.(FunctionCall)
			if !ok {
				t.Fatalf("want FunctionCall, got %T", part)
			}
			if g, w := funcall.Name, weatherTool.FunctionDeclarations[0].Name; g != w {
				t.Errorf("FunctionCall.Name: got %q, want %q", g, w)
			}
			if g, c := funcall.Args["location"], "New York"; !strings.Contains(g.(string), c) {
				t.Errorf(`FunctionCall.Args["location"]: got %q, want string containing %q`, g, c)
			}
			res, err = session.SendMessage(ctx, FunctionResponse{
				Name: weatherTool.FunctionDeclarations[0].Name,
				Response: map[string]any{
					"weather_there": "cold",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			checkMatch(t, responseString(res), "(it's|it is|weather) .*cold")
		})
		t.Run("funcall-none", func(t *testing.T) {
			model.ToolConfig = &ToolConfig{
				FunctionCallingConfig: &FunctionCallingConfig{
					Mode: FunctionCallingNone, // never return a FunctionCall part
				},
			}
			session := model.StartChat()
			res, err := session.SendMessage(ctx, Text("What is the weather like in New York?"))
			if err != nil {
				t.Fatal(err)
			}
			// We should not find a FunctionCall part.
			for _, p := range res.Candidates[0].Content.Parts {
				if _, ok := p.(FunctionCall); ok {
					t.Fatal("saw FunctionCall")
				}
			}
		})
	})
}

func TestLiveREST(t *testing.T) {
	if *projectID == "" || *modelName == "" {
		t.Skip("need -project and -model")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, *projectID, "us-central1", WithREST())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	model := client.GenerativeModel(*modelName)
	model.SetTemperature(0.0)

	resp, err := model.GenerateContent(ctx, Text("What is the average size of a swallow?"))
	if err != nil {
		t.Fatal(err)
	}
	got := responseString(resp)
	checkMatch(t, got, `15.* cm|[1-9].* inches`)
}

func TestJoinResponses(t *testing.T) {
	r1 := &GenerateContentResponse{
		Candidates: []*Candidate{
			{
				Index:        2,
				Content:      &Content{Role: roleModel, Parts: []Part{Text("r1 i2")}},
				FinishReason: FinishReason(1),
			},
			{
				Index:        0,
				Content:      &Content{Role: roleModel, Parts: []Part{Text("r1 i0")}},
				FinishReason: FinishReason(2),
			},
		},
		PromptFeedback: &PromptFeedback{BlockReasonMessage: "br1"},
	}
	r2 := &GenerateContentResponse{
		Candidates: []*Candidate{
			{
				Index:        0,
				Content:      &Content{Role: roleModel, Parts: []Part{Text(";r2 i0")}},
				FinishReason: FinishReason(3),
			},
			{
				// ignored
				Index:        1,
				Content:      &Content{Role: roleModel, Parts: []Part{Text(";r2 i1")}},
				FinishReason: FinishReason(4),
			},
		},

		PromptFeedback: &PromptFeedback{BlockReasonMessage: "br2"},
	}
	got := joinResponses(r1, r2)
	want := &GenerateContentResponse{
		Candidates: []*Candidate{
			{
				Index:        2,
				Content:      &Content{Role: roleModel, Parts: []Part{Text("r1 i2")}},
				FinishReason: FinishReason(1),
			},
			{
				Index:        0,
				Content:      &Content{Role: roleModel, Parts: []Part{Text("r1 i0;r2 i0")}},
				FinishReason: FinishReason(3),
			},
		},
		PromptFeedback: &PromptFeedback{BlockReasonMessage: "br1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\ngot %+v\nwant %+v", got, want)
	}
}

func TestMergeTexts(t *testing.T) {
	for _, test := range []struct {
		in   []Part
		want []Part
	}{
		{
			in:   []Part{Text("a")},
			want: []Part{Text("a")},
		},
		{
			in:   []Part{Text("a"), Text("b"), Text("c")},
			want: []Part{Text("abc")},
		},
		{
			in:   []Part{Blob{"b1", nil}, Text("a"), Text("b"), Blob{"b2", nil}, Text("c")},
			want: []Part{Blob{"b1", nil}, Text("ab"), Blob{"b2", nil}, Text("c")},
		},
		{
			in:   []Part{Text("a"), Text("b"), Blob{"b1", nil}, Text("c"), Text("d"), Blob{"b2", nil}},
			want: []Part{Text("ab"), Blob{"b1", nil}, Text("cd"), Blob{"b2", nil}},
		},
	} {
		got := mergeTexts(test.in)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%+v:\ngot  %+v\nwant %+v", test.in, got, test.want)
		}
	}
}

func checkMatch(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		re, err := regexp.Compile("(?i:" + want + ")")
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(got) {
			t.Errorf("\ngot %q\nwanted to match %q", got, want)
		}
	}
}

func responseString(resp *GenerateContentResponse) string {
	var b strings.Builder
	for i, cand := range resp.Candidates {
		if len(resp.Candidates) > 1 {
			fmt.Fprintf(&b, "%d:", i+1)
		}
		b.WriteString(contentString(cand.Content))
	}
	return b.String()
}

func contentString(c *Content) string {
	var b strings.Builder
	for i, part := range c.Parts {
		if i > 0 {
			fmt.Fprintf(&b, ";")
		}
		fmt.Fprintf(&b, "%v", part)
	}
	return strings.ReplaceAll(b.String(), "\n", " ")
}

func responsesString(t *testing.T, iter *GenerateContentResponseIterator) string {
	var lines []string
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, responseString(resp))
	}
	return strings.Join(lines, "\n")
}

func all(iter *GenerateContentResponseIterator) ([]*GenerateContentResponse, error) {
	var rs []*GenerateContentResponse
	for {
		r, err := iter.Next()
		if err == iterator.Done {
			return rs, nil
		}
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
}

func dump(w io.Writer, x any) {
	var err error
	printf := func(format string, args ...any) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, args...)
		}
	}
	printValue(reflect.ValueOf(x), "", "", printf)
	if err != nil {
		log.Fatal(err)
	}
}

func printValue(v reflect.Value, indent, first string, printf func(string, ...any)) {
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		printf("%s%s%s{\n", indent, first, v.Type())
		indent1 := indent + "    "
		switch v.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				printValue(v.Index(i), indent1, fmt.Sprintf("[%d]: ", i), printf)
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				printValue(iter.Value(), indent1, fmt.Sprintf("%q: ", iter.Key()), printf)
			}
		case reflect.Struct:
			for _, sf := range reflect.VisibleFields(v.Type()) {
				vf := v.FieldByName(sf.Name)
				if !vf.IsZero() {
					printValue(vf, indent1, sf.Name+": ", printf)
				}
			}
		}
		printf("%s}\n", indent)
	case reflect.Pointer, reflect.Interface:
		printValue(v.Elem(), indent, first, printf)
	case reflect.String:
		printf("%s%s%q\n", indent, first, v)
	default:
		printf("%s%s%v\n", indent, first, v)
	}
}

func TestMatchString(t *testing.T) {
	for _, test := range []struct {
		re, in string
	}{
		{"do not", "I do not have"},
		{"(don't|do not) have", "I do not have"},
		{"(don't|do not) have", "As an AI language model, I do not have physical needs"},
	} {
		re := regexp.MustCompile(test.re)
		if !re.MatchString(test.in) {
			t.Errorf("%q doesn't match %q", test.re, test.in)
		}
	}
}

func TestTemperature(t *testing.T) {
	m := &GenerativeModel{}
	got := m.GenerationConfig.toProto().Temperature
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	m.SetTemperature(0)
	got = m.GenerationConfig.toProto().Temperature
	if got == nil {
		t.Fatal("got nil")
	}
	if g := *got; g != 0 {
		t.Errorf("got %v, want 0", g)
	}
}

func TestIntFloatConversions(t *testing.T) {
	for n, test := range []struct {
		i *int32
		f *float32
	}{
		{nil, nil},
		{Ptr[int32](1), Ptr[float32](1)},
	} {
		t.Run(fmt.Sprintf("int-to-float-%d", n), func(t *testing.T) {
			gotf := int32pToFloat32p(test.i)
			if !reflect.DeepEqual(gotf, test.f) {
				t.Errorf("got %v, want %v", gotf, test.f)
			}
		})
		t.Run(fmt.Sprintf("float-to-int-%d", n), func(t *testing.T) {
			goti := float32pToInt32p(test.f)
			if !reflect.DeepEqual(goti, test.i) {
				t.Errorf("got %v, want %v", goti, test.i)
			}
		})
	}
	goti := float32pToInt32p(Ptr[float32](1.5))
	if !reflect.DeepEqual(goti, Ptr[int32](1)) {
		t.Errorf("got %v, want *1", goti)
	}
}
