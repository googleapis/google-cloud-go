// Copyright 2024 Google LLC
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

package prefixmatcher

import (
	"unicode/utf8"
)

// PrefixMatcher helps find longest prefixes. See [FindPrefixLen].
type PrefixMatcher struct {
	root *trieNode
}

type trieNode struct {
	children map[rune]*trieNode
	final    bool
}

// NewFromSet creates a new [PrefixMatcher] from a set of strings tha represent
// the vocabulary.
func NewFromSet(vocab map[string]bool) *PrefixMatcher {
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
