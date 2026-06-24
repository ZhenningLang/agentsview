package enrich

import (
	"fmt"
	"strings"

	"go.kenn.io/agentsview/internal/db"
)

const systemPrompt = `You summarize AI agent sessions for a local archive.
Return only a JSON object with exactly these fields:
{"title":"short title","summary":"one sentence summary","keywords":["keyword"]}
Use the same language as the session when practical. Do not include markdown.`

func buildPrompt(candidate db.EnrichCandidate, samples []string) (string, string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Session ID: %s\n", candidate.ID)
	fmt.Fprintf(&b, "Project: %s\n", candidate.Project)
	fmt.Fprintf(&b, "Agent: %s\n", candidate.Agent)
	if candidate.FirstMessage != "" {
		fmt.Fprintf(&b, "First message: %s\n", candidate.FirstMessage)
	}
	b.WriteString("\nSampled messages:\n")
	for i, sample := range samples {
		fmt.Fprintf(&b, "%d. %s\n", i+1, sample)
	}
	b.WriteString("\nRequirements:\n")
	b.WriteString("- title: concise, useful in a sidebar, <= 80 characters.\n")
	b.WriteString("- summary: one sentence.\n")
	b.WriteString("- keywords: 3 to 8 short searchable terms, include synonyms or topic words when helpful.\n")
	return systemPrompt, b.String()
}
