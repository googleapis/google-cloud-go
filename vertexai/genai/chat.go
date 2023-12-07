package genai

import (
	"context"
)

type ChatSession struct {
	m       *GenerativeModel
	History []*Content
}

func (m *GenerativeModel) StartChat() *ChatSession {
	return &ChatSession{m: m}
}

func (cs *ChatSession) SendMessage(ctx context.Context, parts ...Part) (*GenerateContentResponse, error) {
	// Call the underlying client with the entire history plus the argument Content.
	cs.History = append(cs.History, newUserContent(parts))
	req := cs.m.newRequest(cs.History...)
	cc := int32(1)
	req.GenerationConfig.CandidateCount = &cc
	resp, err := cs.m.generateContent(ctx, req)
	if err != nil {
		return nil, err
	}
	cs.addToHistory(resp.Candidates)
	return resp, nil
}

func (cs *ChatSession) SendMessageStream(ctx context.Context, parts ...Part) *GenerateContentResponseIterator {
	cs.History = append(cs.History, newUserContent(parts))
	req := cs.m.newRequest(cs.History...)
	var cc int32 = 1
	req.GenerationConfig.CandidateCount = &cc
	streamClient, err := cs.m.c.c.StreamGenerateContent(ctx, req)
	return &GenerateContentResponseIterator{
		sc:  streamClient,
		err: err,
		cs:  cs,
	}
}

// By default, use the first candidate for history. The user can modify that if they want.
func (cs *ChatSession) addToHistory(cands []*Candidate) bool {
	if len(cands) > 0 {
		c := cands[0].Content
		c.Role = roleModel
		cs.History = append(cs.History, c)
		return true
	}
	return false
}
