// Copyright 2023 Google LLC

package genai

import (
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/civil"
	pb "cloud.google.com/go/vertexai/internal/aiplatform/apiv1beta1/aiplatformpb"
)

type HarmCategory int32

const (
	HarmCategoryHateSpeech       = HarmCategory(pb.HarmCategory_HARM_CATEGORY_HATE_SPEECH)
	HarmCategoryDangerousContent = HarmCategory(pb.HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT)
	HarmCategoryHarassment       = HarmCategory(pb.HarmCategory_HARM_CATEGORY_HARASSMENT)
	HarmCategorySexuallyExplicit = HarmCategory(pb.HarmCategory_HARM_CATEGORY_SEXUALLY_EXPLICIT)
)

type HarmBlockThreshold int32

const (
	HarmBlockLowAndAbove    = HarmBlockThreshold(pb.SafetySetting_BLOCK_LOW_AND_ABOVE)
	HarmBlockMediumAndAbove = HarmBlockThreshold(pb.SafetySetting_BLOCK_MEDIUM_AND_ABOVE)
	HarmBlockOnlyHigh       = HarmBlockThreshold(pb.SafetySetting_BLOCK_ONLY_HIGH)
	HarmBlockNone           = HarmBlockThreshold(pb.SafetySetting_BLOCK_NONE)
)

type HarmProbability int32

const (
	HarmProbabilityNegligible = HarmProbability(pb.SafetyRating_NEGLIGIBLE)
	HarmProbabilityLow        = HarmProbability(pb.SafetyRating_LOW)
	HarmProbabilityMedium     = HarmProbability(pb.SafetyRating_MEDIUM)
	HarmProbabilityHigh       = HarmProbability(pb.SafetyRating_HIGH)
)

type FinishReason int32

const (
	FinishReasonUnspecified = FinishReason(pb.Candidate_FINISH_REASON_UNSPECIFIED)
	FinishReasonStop        = FinishReason(pb.Candidate_STOP)
	FinishReasonMaxTokens   = FinishReason(pb.Candidate_MAX_TOKENS)
	FinishReasonSafety      = FinishReason(pb.Candidate_SAFETY)
	FinishReasonRecitation  = FinishReason(pb.Candidate_RECITATION)
	FinishReasonOther       = FinishReason(pb.Candidate_OTHER)
)

var finishReasonStrings = map[FinishReason]string{
	FinishReasonUnspecified: "Unspecified",
	FinishReasonStop:        "Stop",
	FinishReasonMaxTokens:   "MaxTokens",
	FinishReasonSafety:      "Safety",
	FinishReasonRecitation:  "Recitation",
	FinishReasonOther:       "Other",
}

func (f FinishReason) String() string {
	if s, ok := finishReasonStrings[f]; ok {
		return s
	}
	return fmt.Sprintf("FinishReason(%d)", f)
}

func (f FinishReason) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(f.String())), nil
}

const (
	roleUser  = "user"
	roleModel = "model"
)

type Content struct {
	Role  string
	Parts []Part
}

func (c *Content) proto() *pb.Content {
	return &pb.Content{
		Role:  c.Role,
		Parts: mapSlice(c.Parts, Part.proto),
	}
}

func protoToContent(c *pb.Content) *Content {
	return &Content{
		Role:  c.Role,
		Parts: mapSlice(c.Parts, protoToPart),
	}
}

// A part is either a Text, a Blob, or a FileData.
type Part interface {
	proto() *pb.Part
}

func protoToPart(p *pb.Part) Part {
	switch d := p.Data.(type) {
	case *pb.Part_Text:
		return Text(d.Text)
	case *pb.Part_InlineData:
		return Blob{
			MIMEType: d.InlineData.MimeType,
			Data:     d.InlineData.Data,
		}
	case *pb.Part_FileData:
		return FileData{
			MIMEType: d.FileData.MimeType,
			FileURI:  d.FileData.FileUri,
		}
	default:
		panic(fmt.Errorf("unknown Part.Data type %T", p.Data))
	}
}

type Text string

func (t Text) proto() *pb.Part {
	return &pb.Part{
		Data: &pb.Part_Text{Text: string(t)},
	}
}

type Blob struct {
	MIMEType string
	Data     []byte
}

func (b Blob) proto() *pb.Part {
	return &pb.Part{
		Data: &pb.Part_InlineData{
			InlineData: &pb.Blob{
				MimeType: b.MIMEType,
				Data:     b.Data,
			},
		},
	}
}

type FileData struct {
	MIMEType string
	FileURI  string
}

func (f FileData) proto() *pb.Part {
	return &pb.Part{
		Data: &pb.Part_FileData{
			FileData: &pb.FileData{
				MimeType: f.MIMEType,
				FileUri:  f.FileURI,
			},
		},
	}
}

// ImageData is a convenience function for creating an image
// Blob for input to a model.
// The format should be the second part of the MIME type, after "image/".
// For example, for a PNG image, pass "png".
func ImageData(format string, data []byte) Blob {
	return Blob{
		MIMEType: "image/" + format,
		Data:     data,
	}
}

type GenerationConfig struct {
	Temperature      float32
	TopP             float32 // if non-zero, use nucleus sampling
	TopK             float32 // if non-zero, use top-K sampling
	CandidateCount   int32
	MaxOutputTokens  int32
	StopSequences    []string
	Logprobs         int32
	PresencePenalty  float32
	FrequencyPenalty float32
	LogitBias        map[string]float32
	Echo             bool
}

func (c *GenerationConfig) proto() *pb.GenerationConfig {
	return &pb.GenerationConfig{
		Temperature:     &c.Temperature,
		TopP:            &c.TopP,
		TopK:            &c.TopK,
		CandidateCount:  &c.CandidateCount,
		MaxOutputTokens: &c.MaxOutputTokens,
		StopSequences:   c.StopSequences,
	}
}

type SafetySetting struct {
	Category  HarmCategory
	Threshold HarmBlockThreshold
}

func (s *SafetySetting) proto() *pb.SafetySetting {
	return &pb.SafetySetting{
		Category:  pb.HarmCategory(s.Category),
		Threshold: pb.SafetySetting_HarmBlockThreshold(s.Threshold),
	}
}

type SafetyRating struct {
	Category    HarmCategory
	Probability HarmProbability
	Blocked     bool
}

func protoToSafetyRating(r *pb.SafetyRating) *SafetyRating {
	return &SafetyRating{
		Category:    HarmCategory(r.Category),
		Probability: HarmProbability(r.Probability),
		Blocked:     r.Blocked,
	}
}

type CitationMetadata struct {
	Citations []*Citation
}

func protoToCitationMetadata(cm *pb.CitationMetadata) *CitationMetadata {
	if cm == nil {
		return nil
	}
	return &CitationMetadata{
		Citations: mapSlice(cm.Citations, protoToCitation),
	}
}

type Citation struct {
	StartIndex, EndIndex int32
	URI                  string
	Title                string
	License              string
	PublicationDate      civil.Date
}

func protoToCitation(c *pb.Citation) *Citation {
	r := &Citation{
		StartIndex: c.StartIndex,
		EndIndex:   c.EndIndex,
		URI:        c.Uri,
		Title:      c.Title,
		License:    c.License,
	}
	if c.PublicationDate != nil {
		r.PublicationDate = civil.Date{
			Year:  int(c.PublicationDate.Year),
			Month: time.Month(c.PublicationDate.Month),
			Day:   int(c.PublicationDate.Day),
		}
	}
	return r
}

type Candidate struct {
	Index        int32
	Content      *Content
	FinishReason FinishReason
	//FinishMessage    string
	SafetyRatings    []*SafetyRating
	CitationMetadata *CitationMetadata
}

func protoToCandidate(c *pb.Candidate) *Candidate {
	// TODO: confirm that there is no difference between an empty FinishMessage an a nil one.
	// fm := ""
	// if c.FinishMessage != nil {
	// 	fm = *c.FinishMessage
	// }
	return &Candidate{
		Index:         c.Index,
		Content:       protoToContent(c.Content),
		FinishReason:  FinishReason(c.FinishReason),
		SafetyRatings: mapSlice(c.SafetyRatings, protoToSafetyRating),
		//FinishMessage:    fm,
		CitationMetadata: protoToCitationMetadata(c.CitationMetadata),
	}
}
