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

// To get the protoveneer tool:
//    go install golang.org/x/exp/protoveneer/cmd/protoveneer@latest

//go:generate protoveneer -license license.txt config.yaml ../../aiplatform/apiv1beta1/aiplatformpb

// Package genai is a client for the generative VertexAI model.
package genai

import (
	"context"
	"fmt"
	"io"
	"strings"

	aiplatform "cloud.google.com/go/aiplatform/apiv1beta1"
	pb "cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"cloud.google.com/go/vertexai/internal"
	"cloud.google.com/go/vertexai/internal/support"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// A Client is a Google Vertex AI client.
type Client struct {
	c         *aiplatform.PredictionClient
	projectID string
	location  string
}

// NewClient creates a new Google Vertex AI client.
//
// Clients should be reused instead of created as needed. The methods of Client
// are safe for concurrent use by multiple goroutines.
// projectID is your GCP project; location is GCP region/location per
// https://cloud.google.com/vertex-ai/docs/general/locations
//
// You may configure the client by passing in options from the
// [google.golang.org/api/option] package. You may also use options defined in
// this package, such as [WithREST].
func NewClient(ctx context.Context, projectID, location string, opts ...option.ClientOption) (*Client, error) {
	opts = append([]option.ClientOption{
		option.WithEndpoint(fmt.Sprintf("%s-aiplatform.googleapis.com:443", location)),
	}, opts...)
	conf := newConfig(opts...)

	var c *aiplatform.PredictionClient
	var err error
	if conf.withREST {
		c, err = aiplatform.NewPredictionRESTClient(ctx, opts...)
	} else {
		c, err = aiplatform.NewPredictionClient(ctx, opts...)
	}
	if err != nil {
		return nil, err
	}

	c.SetGoogleClientInfo("gccl", internal.Version)
	return &Client{
		c:         c,
		projectID: projectID,
		location:  location,
	}, nil
}

// Close closes the client.
func (c *Client) Close() error {
	return c.c.Close()
}

// GenerativeModel is a model that can generate text.
// Create one with [Client.GenerativeModel], then configure
// it by setting the exported fields.
//
// The model holds all the config for a GenerateContentRequest, so the GenerateContent method
// can use a vararg for the content.
type GenerativeModel struct {
	c        *Client
	name     string
	fullName string

	GenerationConfig
	SafetySettings    []*SafetySetting
	Tools             []*Tool
	ToolConfig        *ToolConfig // configuration for tools
	SystemInstruction *Content
}

const defaultMaxOutputTokens = 2048

// GenerativeModel creates a new instance of the named model.
// name is a string model name like "gemini-1.0.-pro".
// See https://cloud.google.com/vertex-ai/generative-ai/docs/learn/model-versioning
// for details on model naming and versioning.
func (c *Client) GenerativeModel(name string) *GenerativeModel {
	return &GenerativeModel{
		c:        c,
		name:     name,
		fullName: fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s", c.projectID, c.location, name),
	}
}

// Name returns the name of the model.
func (m *GenerativeModel) Name() string {
	return m.name
}

// GenerateContent produces a single request and response.
func (m *GenerativeModel) GenerateContent(ctx context.Context, parts ...Part) (*GenerateContentResponse, error) {
	return m.generateContent(ctx, m.newGenerateContentRequest(newUserContent(parts)))
}

// GenerateContentStream returns an iterator that enumerates responses.
func (m *GenerativeModel) GenerateContentStream(ctx context.Context, parts ...Part) *GenerateContentResponseIterator {
	streamClient, err := m.c.c.StreamGenerateContent(ctx, m.newGenerateContentRequest(newUserContent(parts)))
	return &GenerateContentResponseIterator{
		sc:  streamClient,
		err: err,
	}
}

func (m *GenerativeModel) generateContent(ctx context.Context, req *pb.GenerateContentRequest) (*GenerateContentResponse, error) {
	res, err := m.c.c.GenerateContent(ctx, req)

	if err != nil {
		return nil, err
	}
	return protoToResponse(res)
}

func (m *GenerativeModel) newGenerateContentRequest(contents ...*Content) *pb.GenerateContentRequest {
	return &pb.GenerateContentRequest{
		Model:             m.fullName,
		Contents:          support.TransformSlice(contents, (*Content).toProto),
		SafetySettings:    support.TransformSlice(m.SafetySettings, (*SafetySetting).toProto),
		Tools:             support.TransformSlice(m.Tools, (*Tool).toProto),
		ToolConfig:        m.ToolConfig.toProto(),
		GenerationConfig:  m.GenerationConfig.toProto(),
		SystemInstruction: m.SystemInstruction.toProto(),
	}
}

func newUserContent(parts []Part) *Content {
	return &Content{Role: roleUser, Parts: parts}
}

// GenerateContentResponseIterator is an iterator over GnerateContentResponse.
type GenerateContentResponseIterator struct {
	sc     pb.PredictionService_StreamGenerateContentClient
	err    error
	merged *GenerateContentResponse
	cs     *ChatSession
}

// Next returns the next response.
func (iter *GenerateContentResponseIterator) Next() (*GenerateContentResponse, error) {
	if iter.err != nil {
		return nil, iter.err
	}
	resp, err := iter.sc.Recv()
	iter.err = err
	if err == io.EOF {
		if iter.cs != nil && iter.merged != nil {
			iter.cs.addToHistory(iter.merged.Candidates)
		}
		return nil, iterator.Done
	}
	if err != nil {
		return nil, err
	}
	gcp, err := protoToResponse(resp)
	if err != nil {
		iter.err = err
		return nil, err
	}
	// Merge this response in with the ones we've already seen.
	iter.merged = joinResponses(iter.merged, gcp)
	// If this is part of a ChatSession, remember the response for the history.
	return gcp, nil
}

func protoToResponse(resp *pb.GenerateContentResponse) (*GenerateContentResponse, error) {
	gcp := (GenerateContentResponse{}).fromProto(resp)
	// Assume a non-nil PromptFeedback is an error.
	// TODO: confirm.
	if gcp.PromptFeedback != nil {
		return nil, &BlockedError{PromptFeedback: gcp.PromptFeedback}
	}
	// If any candidate is blocked, error.
	// TODO: is this too harsh?
	for _, c := range gcp.Candidates {
		if c.FinishReason == FinishReasonSafety {
			return nil, &BlockedError{Candidate: c}
		}
	}
	return gcp, nil
}

// CountTokens counts the number of tokens in the content.
func (m *GenerativeModel) CountTokens(ctx context.Context, parts ...Part) (*CountTokensResponse, error) {
	req := m.newCountTokensRequest(newUserContent(parts))
	res, err := m.c.c.CountTokens(ctx, req)
	if err != nil {
		return nil, err
	}

	return (CountTokensResponse{}).fromProto(res), nil
}

func (m *GenerativeModel) newCountTokensRequest(contents ...*Content) *pb.CountTokensRequest {
	return &pb.CountTokensRequest{
		Endpoint: m.fullName,
		Model:    m.fullName,
		Contents: support.TransformSlice(contents, (*Content).toProto),
	}
}

// A BlockedError indicates that the model's response was blocked.
// There can be two underlying causes: the prompt or a candidate response.
type BlockedError struct {
	// If non-nil, the model's response was blocked.
	// Consult the Candidate and SafetyRatings fields for details.
	Candidate *Candidate

	// If non-nil, there was a problem with the prompt.
	PromptFeedback *PromptFeedback
}

func (e *BlockedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "blocked: ")
	if e.Candidate != nil {
		fmt.Fprintf(&b, "candidate: %s", e.Candidate.FinishReason)
	}
	if e.PromptFeedback != nil {
		if e.Candidate != nil {
			fmt.Fprintf(&b, ", ")
		}
		fmt.Fprintf(&b, "prompt: %v (%s)", e.PromptFeedback.BlockReason, e.PromptFeedback.BlockReasonMessage)
	}
	return b.String()
}

// joinResponses  merges the two responses, which should be the result of a streaming call.
// The first argument is modified.
func joinResponses(dest, src *GenerateContentResponse) *GenerateContentResponse {
	if dest == nil {
		return src
	}
	dest.Candidates = joinCandidateLists(dest.Candidates, src.Candidates)
	// Keep dest.PromptFeedback.
	// TODO: Take the last UsageMetadata.
	return dest
}

func joinCandidateLists(dest, src []*Candidate) []*Candidate {
	indexToSrcCandidate := map[int32]*Candidate{}
	for _, s := range src {
		indexToSrcCandidate[s.Index] = s
	}
	for _, d := range dest {
		s := indexToSrcCandidate[d.Index]
		if s != nil {
			d.Content = joinContent(d.Content, s.Content)
			// Take the last of these.
			d.FinishReason = s.FinishReason
			d.FinishMessage = s.FinishMessage
			d.SafetyRatings = s.SafetyRatings
			d.CitationMetadata = joinCitationMetadata(d.CitationMetadata, s.CitationMetadata)
		}
	}
	return dest
}

func joinCitationMetadata(dest, src *CitationMetadata) *CitationMetadata {
	if dest == nil {
		return src
	}
	if src == nil {
		return dest
	}
	dest.Citations = append(dest.Citations, src.Citations...)
	return dest
}

func joinContent(dest, src *Content) *Content {
	if dest == nil {
		return src
	}
	if src == nil {
		return dest
	}
	// Assume roles are the same.
	dest.Parts = joinParts(dest.Parts, src.Parts)
	return dest
}

func joinParts(dest, src []Part) []Part {
	return mergeTexts(append(dest, src...))
}

func mergeTexts(in []Part) []Part {
	var out []Part
	i := 0
	for i < len(in) {
		if t, ok := in[i].(Text); ok {
			texts := []string{string(t)}
			var j int
			for j = i + 1; j < len(in); j++ {
				if t, ok := in[j].(Text); ok {
					texts = append(texts, string(t))
				} else {
					break
				}
			}
			// j is just after the last Text.
			out = append(out, Text(strings.Join(texts, "")))
			i = j
		} else {
			out = append(out, in[i])
			i++
		}
	}
	return out
}

func int32pToFloat32p(x *int32) *float32 {
	if x == nil {
		return nil
	}
	f := float32(*x)
	return &f
}

func float32pToInt32p(x *float32) *int32 {
	if x == nil {
		return nil
	}
	i := int32(*x)
	return &i
}
