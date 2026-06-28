package extract

import (
	"fmt"
	"strings"

	"go.kenn.io/agentsview/internal/db"
)

const systemPrompt = `You extract durable memory candidates from agent sessions.
Return JSON only: {"candidates":[{"category":"decision|correction|preference|failure-mode|fact|knowledge|pattern|bug","summary":"...","why":"...","evidence":"...","implication":"..."}]}.
Write summary, why, evidence, and implication in the SAME natural language as the conversation content — use 中文 when the conversation is mostly Chinese, English when it is mostly English. Keep the JSON keys and category values in English.
Only include information useful for future sessions. For decision candidates, why is required. Do not include secrets.`

func BuildUserPrompt(sess db.Session, msgs []db.Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Session ID: %s\n", sess.ID)
	fmt.Fprintf(&b, "Agent: %s\n", sess.Agent)
	fmt.Fprintf(&b, "Project: %s\n\n", sess.Project)
	for _, msg := range msgs {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if len(content) > 2000 {
			content = content[:2000]
		}
		fmt.Fprintf(&b, "[%s]\n%s\n\n", role, content)
	}
	return b.String()
}
