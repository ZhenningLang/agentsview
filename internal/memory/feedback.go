package memory

import (
	"sort"
	"strings"
)

// YAMLQuote renders a free-text value as a YAML double-quoted scalar.
func YAMLQuote(s string) string { return yamlQuote(s) }

// SetFrontmatterFields upserts key/value lines in the first YAML frontmatter block.
func SetFrontmatterFields(content string, kv map[string]string) string {
	return setFrontmatterFields(content, kv)
}

func yamlQuote(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return `"` + s + `"`
}

func setFrontmatterFields(content string, kv map[string]string) string {
	if len(kv) == 0 {
		return content
	}
	keys := orderedFrontmatterKeys(kv)
	fieldLines := make([]string, 0, len(keys))
	for _, key := range keys {
		fieldLines = append(fieldLines, key+": "+kv[key])
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "---\n" + strings.Join(fieldLines, "\n") + "\n---\n\n" + content
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return "---\n" + strings.Join(fieldLines, "\n") + "\n---\n\n" + content
	}

	front := append([]string(nil), lines[:closeIdx]...)
	replaced := make(map[string]bool, len(kv))
	for i, line := range front {
		for _, key := range keys {
			if frontmatterLineHasKey(line, key) {
				front[i] = key + ": " + kv[key]
				replaced[key] = true
				break
			}
		}
	}
	for _, key := range keys {
		if !replaced[key] {
			front = append(front, key+": "+kv[key])
		}
	}

	out := append(front, lines[closeIdx:]...)
	return strings.Join(out, "\n")
}

func frontmatterLineHasKey(line, key string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	idx := strings.Index(trimmed, ":")
	if idx == -1 {
		return false
	}
	return strings.TrimSpace(trimmed[:idx]) == key
}

func orderedFrontmatterKeys(kv map[string]string) []string {
	preferred := []string{"feedback_vote", "feedback_comment", "feedback_status"}
	seen := make(map[string]bool, len(kv))
	keys := make([]string, 0, len(kv))
	for _, key := range preferred {
		if _, ok := kv[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range kv {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}
