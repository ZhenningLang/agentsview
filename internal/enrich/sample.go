package enrich

import (
	"strings"
	"unicode/utf8"

	"go.kenn.io/agentsview/internal/db"
)

const (
	maxSampleMessages = 10
	maxSampleRunes    = 500
)

func sampleMessages(messages []db.Message) []string {
	filtered := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.IsSystem || !isVisibleRole(msg.Role) {
			continue
		}
		content := normalizeSampleContent(msg.Content)
		if content == "" || isNoise(content) {
			continue
		}
		filtered = append(filtered, truncateRunes(content, maxSampleRunes))
	}
	if len(filtered) <= maxSampleMessages {
		return filtered
	}
	selected := append([]string{}, filtered[:3]...)
	middle := filtered[3 : len(filtered)-3]
	if len(middle) <= 4 {
		selected = append(selected, middle...)
	} else {
		step := float64(len(middle)-1) / 3
		last := -1
		for i := 0; i < 4; i++ {
			idx := int(float64(i)*step + 0.5)
			if idx <= last {
				idx = last + 1
			}
			if idx >= len(middle) {
				idx = len(middle) - 1
			}
			selected = append(selected, middle[idx])
			last = idx
		}
	}
	selected = append(selected, filtered[len(filtered)-3:]...)
	return selected
}

func isVisibleRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "assistant":
		return true
	default:
		return false
	}
}

func normalizeSampleContent(content string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
}

func isNoise(content string) bool {
	trimmed := strings.TrimSpace(content)
	if utf8.RuneCountInString(trimmed) < 5 {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "已取消") || strings.Contains(lower, "<system-reminder>") {
		return true
	}
	for _, prefix := range db.SystemMsgPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func truncateRunes(content string, limit int) string {
	if utf8.RuneCountInString(content) <= limit {
		return content
	}
	runes := []rune(content)
	return string(runes[:limit])
}
