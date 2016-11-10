// Copyright 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package language

import (
	languagepb "google.golang.org/genproto/googleapis/cloud/language/v1"
)

import (
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockLanguageService struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ languagepb.LanguageServiceServer = &mockLanguageService{}

func (s *mockLanguageService) AnalyzeSentiment(_ context.Context, req *languagepb.AnalyzeSentimentRequest) (*languagepb.AnalyzeSentimentResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnalyzeSentimentResponse), nil
}

func (s *mockLanguageService) AnalyzeEntities(_ context.Context, req *languagepb.AnalyzeEntitiesRequest) (*languagepb.AnalyzeEntitiesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnalyzeEntitiesResponse), nil
}

func (s *mockLanguageService) AnalyzeSyntax(_ context.Context, req *languagepb.AnalyzeSyntaxRequest) (*languagepb.AnalyzeSyntaxResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnalyzeSyntaxResponse), nil
}

func (s *mockLanguageService) AnnotateText(_ context.Context, req *languagepb.AnnotateTextRequest) (*languagepb.AnnotateTextResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*languagepb.AnnotateTextResponse), nil
}
