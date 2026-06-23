package skills

import "unicode"

// Tokenizer estimates the token count of a text. It is deliberately
// pluggable: the default implementation is a dependency-free heuristic,
// but it can be swapped for a real BPE tokenizer (e.g. tiktoken) later
// without touching the syncer or schema.
type Tokenizer interface {
	// Name identifies the tokenizer for provenance (stored per skill).
	Name() string
	// Count returns an estimated token count for text.
	Count(text string) int
}

// heuristicTokenizer approximates token counts without any external
// dependency. It is NOT the Anthropic tokenizer; counts are approximate
// and intended only to size resident description cost to an order of
// magnitude (the C3 goal is "is this <1% of the window?", not exact
// billing). CJK runes are weighted higher than Latin characters because
// modern BPE vocabularies emit roughly one token per one-to-two CJK
// characters but pack several Latin characters per token.
type heuristicTokenizer struct{}

// NewHeuristicTokenizer returns the default approximate tokenizer.
func NewHeuristicTokenizer() Tokenizer { return heuristicTokenizer{} }

func (heuristicTokenizer) Name() string { return "heuristic-v1" }

func (heuristicTokenizer) Count(text string) int {
	var cjk, other float64
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			// Whitespace rarely produces standalone tokens.
			continue
		case isCJK(r):
			cjk++
		default:
			other++
		}
	}
	// Calibrated so the full 52-skill Chinese catalog lands in the
	// ~1400-2000 token range observed in practice.
	tokens := cjk*0.6 + other*0.25
	if tokens > 0 && tokens < 1 {
		return 1
	}
	return int(tokens + 0.5)
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // full-width forms
}
