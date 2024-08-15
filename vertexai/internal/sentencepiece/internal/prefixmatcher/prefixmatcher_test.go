package prefixmatcher

import (
	"fmt"
	"testing"
)

func dumpNode(n *trieNode, prefix string) string {
	var s string
	if n.final {
		s = fmt.Sprintf("%sfinal\n", prefix)
	}
	for r, c := range n.children {
		s += fmt.Sprintf("%s%q ->\n%s", prefix, r, dumpNode(c, prefix+"  "))
	}
	return s
}

func TestSmallVocab(t *testing.T) {
	vocab := map[string]struct{}{
		"ham":    struct{}{},
		"yefet":  struct{}{},
		"hamat":  struct{}{},
		"hamela": struct{}{},
		"世界":     struct{}{},

		"▁▁":     struct{}{},
		"▁▁▁":    struct{}{},
		"▁▁▁▁":   struct{}{},
		"▁▁▁▁▁":  struct{}{},
		"▁▁▁▁▁▁": struct{}{},
	}
	pm := NewFromSet(vocab)

	var tests = []struct {
		text    string
		wantLen int
	}{
		{"zyx", 0},
		{"ham", 3},
		{"hama", 3},
		{"zham", 0},
		{"hame", 3},
		{"hamy", 3},
		{"hamat", 5},
		{"hamatar", 5},
		{"hamela", 6},
		{"hamelar", 6},
		{"y", 0},
		{"ye", 0},
		{"yefet", 5},
		{"yefeton", 5},
		{"世界", 6},
		{"世", 0},
		{"世p", 0},
		{"世界foo", 6},
		{"▁", 0},
		{"▁▁", 6},
		{"▁▁▁", 9},
		{"▁▁▁▁", 12},
		{"▁▁▁▁▁", 15},
		{"▁▁▁▁▁▁", 18},
		{"▁▁▁▁▁▁▁", 18},
		{"▁▁▁▁▁▁p", 18},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			gotLen := pm.FindPrefixLen(tt.text)
			if gotLen != tt.wantLen {
				t.Errorf("got %v, want %v", gotLen, tt.wantLen)
			}
		})
	}
}

func TestSingleAndDoubleLetter(t *testing.T) {
	vocab := make(map[string]struct{})

	for r1 := 'a'; r1 <= 'z'; r1++ {
		vocab[string(r1)] = struct{}{}

		for r2 := 'a'; r2 <= 'z'; r2++ {
			vocab[string(r1)+string(r2)] = struct{}{}
		}
	}

	pm := NewFromSet(vocab)

	assertLen := func(text string, wantLen int) {
		t.Helper()
		gotLen := pm.FindPrefixLen(text)
		if gotLen != wantLen {
			t.Errorf("got %v, want %v", gotLen, wantLen)
		}
	}

	for r1 := 'a'; r1 <= 'z'; r1++ {
		assertLen(string(r1), 1)
		for r2 := 'a'; r2 <= 'z'; r2++ {
			assertLen(string(r1)+string(r2), 2)
			for r3 := 'a'; r3 <= 'z'; r3++ {
				assertLen(string(r1)+string(r2)+string(r3), 2)
			}
		}
	}
}
