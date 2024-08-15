package sentencepiece

import "fmt"

// Token represents a single token from the input text. ID is a unique token
// identifier that the model uses in its internal representation. Text is
// the piece of text this token represents.
type Token struct {
	ID   int
	Text string
}

func (t Token) String() string {
	return fmt.Sprintf("Token{ID: %v, Text: %q}", t.ID, t.Text)
}
