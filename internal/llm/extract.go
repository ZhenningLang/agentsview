package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Enrichment struct {
	Title    string
	Summary  string
	Keywords []string
}

func ParseEnrichment(jsonText string) (Enrichment, error) {
	object, err := extractJSONObject(jsonText)
	if err != nil {
		return Enrichment{}, err
	}
	var raw struct {
		Title    string          `json:"title"`
		Summary  string          `json:"summary"`
		Keywords json.RawMessage `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(object), &raw); err != nil {
		return Enrichment{}, fmt.Errorf("parse enrichment: %w", err)
	}
	result := Enrichment{
		Title:   strings.TrimSpace(raw.Title),
		Summary: strings.TrimSpace(raw.Summary),
	}
	if result.Title == "" {
		return Enrichment{}, fmt.Errorf("parse enrichment: title is required")
	}
	result.Keywords, err = parseKeywords(raw.Keywords)
	if err != nil {
		return Enrichment{}, err
	}
	return result, nil
}

func extractJSONObject(text string) (string, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.IndexByte(text, '{')
	if start < 0 {
		return "", fmt.Errorf("parse enrichment: missing JSON object")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("parse enrichment: unterminated JSON object")
}

func parseKeywords(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return normalizeKeywords(values), nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("parse enrichment: invalid keywords")
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	})
	return normalizeKeywords(parts), nil
}

func normalizeKeywords(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
