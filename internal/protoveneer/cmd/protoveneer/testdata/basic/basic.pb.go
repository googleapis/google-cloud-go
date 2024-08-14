// Copyright 2024 Google LLC
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

package basic

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	status "google.golang.org/genproto/googleapis/rpc/status"
	date "google.golang.org/genproto/googleapis/type/date"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Harm categories that will block the content.
type HarmCategory int32

const (
	// The harm category is unspecified.
	HarmCategory_HARM_CATEGORY_UNSPECIFIED HarmCategory = 0
	// The harm category is hate speech.
	HarmCategory_HARM_CATEGORY_HATE_SPEECH HarmCategory = 1
	// The harm category is dangerous content.
	HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT HarmCategory = 2
	// The harm category is harassment.
	HarmCategory_HARM_CATEGORY_HARASSMENT HarmCategory = 3
	// The harm category is sexually explicit content.
	HarmCategory_HARM_CATEGORY_SEXUALLY_EXPLICIT HarmCategory = 4
)

// The reason why the model stopped generating tokens.
// If empty, the model has not stopped generating the tokens.
type Candidate_FinishReason int32

const (
	// The finish reason is unspecified.
	Candidate_FINISH_REASON_UNSPECIFIED Candidate_FinishReason = 0
	// Natural stop point of the model or provided stop sequence.
	Candidate_STOP Candidate_FinishReason = 1
	// The maximum number of tokens as specified in the request was reached.
	Candidate_MAX_TOKENS Candidate_FinishReason = 2
	// The token generation was stopped as the response was flagged for safety
	// reasons. NOTE: When streaming the Candidate.content will be empty if
	// content filters blocked the output.
	Candidate_SAFETY Candidate_FinishReason = 3
	// The token generation was stopped as the response was flagged for
	// unauthorized citations.
	Candidate_RECITATION Candidate_FinishReason = 4
	// All other reasons that stopped the token generation
	Candidate_OTHER Candidate_FinishReason = 5
)

// Raw media bytes.
//
// Text should not be sent as raw bytes, use the 'text' field.
type Blob struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Required. The IANA standard MIME type of the source data.
	MimeType string `protobuf:"bytes,1,opt,name=mime_type,json=mimeType,proto3" json:"mime_type,omitempty"`
	// Required. Raw bytes for media formats.
	Data []byte `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
}

// Generation config.
type GenerationConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Optional. Controls the randomness of predictions.
	Temperature *float32 `protobuf:"fixed32,1,opt,name=temperature,proto3,oneof" json:"temperature,omitempty"`
	// Optional. Number of candidates to generate.
	CandidateCount *int32 `protobuf:"varint,4,opt,name=candidate_count,json=candidateCount,proto3,oneof" json:"candidate_count,omitempty"`
	// Optional. Stop sequences.
	StopSequences []string `protobuf:"bytes,6,rep,name=stop_sequences,json=stopSequences,proto3" json:"stop_sequences,omitempty"`
	HarmCat       HarmCategory
	// Bad doc that doesn't make sense.
	FinishReason Candidate_FinishReason
	CitMet       *CitationMetadata
	TopK         *float32
}

// A collection of source attributions for a piece of content.
type CitationMetadata struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Output only. List of citations.
	Citations []*Citation `protobuf:"bytes,1,rep,name=citations,proto3" json:"citations,omitempty"`
	CitMap    map[string]*Citation
}

// Source attributions for content.
type Citation struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Output only. Url reference of the attribution.
	Uri string `protobuf:"bytes,3,opt,name=uri,proto3" json:"uri,omitempty"`
	// Output only. Publication date of the attribution.
	PublicationDate *date.Date `protobuf:"bytes,6,opt,name=publication_date,json=publicationDate,proto3" json:"publication_date,omitempty"`
	Struct          *structpb.Struct
	CreateTime      *timestamppb.Timestamp
}

type unexported interface{ u() }

// This demonstrates using population functions to deal with
// proto oneof field, which has an unexported type.
// That can be a way to deal with proto oneofs.
type Pop struct {
	X int
	Y unexported
}

// status.Status converts to apierror.APIError.
// durationpb.Duration converts to time.Duration.

type File struct {
	Error *status.Status
	Dur   *durationpb.Duration
}
