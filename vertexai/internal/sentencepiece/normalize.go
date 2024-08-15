package sentencepiece

import "strings"

// normalize performs unicode normalization.
//
// SentencePiece has a feature to perform configurable unicode normalization on
// the input text and has some options for adding dummy whitespace prefixes or
// trimming whitespace. However, the model we're working with has a very simple
// normalizer that does none of this. These options can be added in the future
// if needed.
func normalize(text string) string {
	return replaceSeparator(text)
}

// replaceSeparator replaces spaces by the whitespace separator used by
// the model.
func replaceSeparator(text string) string {
	return strings.ReplaceAll(text, " ", "▁")
}
