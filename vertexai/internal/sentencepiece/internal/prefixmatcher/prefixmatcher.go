package prefixmatcher

import (
	"unicode/utf8"
)

type PrefixMatcher struct {
	root *trieNode
}

type trieNode struct {
	children map[rune]*trieNode
	final    bool
}

func NewFromSet(vocab map[string]struct{}) *PrefixMatcher {
	pm := &PrefixMatcher{root: newNode()}
	for word := range vocab {
		pm.add(word)
	}
	return pm
}

// FindPrefixLen finds the longest prefix of text that matches a vocabulary
// word, and returns it. If 0 is returned, no prefix was found.
func (pm *PrefixMatcher) FindPrefixLen(text string) int {
	node := pm.root
	maxLen := 0

	for i, r := range text {
		child := node.children[r]
		if child == nil {
			// r not found in this node, so we're done.
			return maxLen
		}
		if child.final {
			maxLen = i + utf8.RuneLen(r)
		}
		node = child
	}

	return maxLen
}

func (pm *PrefixMatcher) add(word string) {
	node := pm.root

	for _, r := range word {
		child := node.children[r]
		if child == nil {
			child = newNode()
			node.children[r] = child
		}
		node = child
	}

	node.final = true
}

func newNode() *trieNode {
	return &trieNode{
		children: make(map[rune]*trieNode),
		final:    false,
	}
}
