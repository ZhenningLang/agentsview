package memory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYAMLQuoteEscapesFeedbackComment(t *testing.T) {
	got := yamlQuote("  原因: \"过度\\合并\"\n第二行\r  ")
	assert.Equal(t, `"原因: \"过度\\合并\" 第二行"`, got)

	var parsed string
	require.NoError(t, yaml.Unmarshal([]byte(got), &parsed))
	assert.Equal(t, `原因: "过度\合并" 第二行`, parsed)
}

func TestSetFrontmatterFieldsInsertsMissingKeys(t *testing.T) {
	content := "---\ntitle: Alpha\n---\n\nBody\n"
	got := setFrontmatterFields(content, map[string]string{
		"feedback_vote":   "down",
		"feedback_status": "pending",
	})

	assert.Equal(t,
		"---\ntitle: Alpha\nfeedback_vote: down\nfeedback_status: pending\n---\n\nBody\n",
		got)
}

func TestSetFrontmatterFieldsReplacesExistingKeysOnlyInFrontmatter(t *testing.T) {
	content := strings.Join([]string{
		"---",
		"title: Alpha",
		"feedback_vote: down",
		"feedback_voteX: keep",
		"---",
		"",
		"feedback_vote: body must stay",
		"note: x",
		"",
	}, "\n")

	got := setFrontmatterFields(content, map[string]string{"feedback_vote": "up"})

	assert.Contains(t, got, "feedback_vote: up\nfeedback_voteX: keep")
	assert.Contains(t, got, "feedback_vote: body must stay")
	assert.Contains(t, got, "note: x")
	assert.NotContains(t, got, "feedback_vote: down")
}

func TestSetFrontmatterFieldsReplacesKeyWithSpaceBeforeColon(t *testing.T) {
	content := "---\ntitle: Alpha\nfeedback_vote : down\n---\n\nBody\n"
	got := setFrontmatterFields(content, map[string]string{"feedback_vote": "up"})
	block := extractFrontmatterBlock(got)

	assert.Contains(t, got, "feedback_vote: up")
	assert.NotContains(t, got, "feedback_vote : down")
	assert.Equal(t, 1, strings.Count(block, "feedback_vote"))
	var fm map[string]string
	require.NoError(t, yaml.Unmarshal([]byte(block), &fm))
	assert.Equal(t, "up", fm["feedback_vote"])
}

func TestSetFrontmatterFieldsReplacesExistingCommentWithQuotedValue(t *testing.T) {
	content := "---\ntitle: Alpha\nfeedback_comment: \"old\"\n---\n\nBody\n"
	got := setFrontmatterFields(content, map[string]string{
		"feedback_comment": yamlQuote("原因: 新评论"),
	})
	block := extractFrontmatterBlock(got)

	assert.NotContains(t, got, `feedback_comment: "old"`)
	assert.Equal(t, 1, strings.Count(block, "feedback_comment"))
	var fm map[string]string
	require.NoError(t, yaml.Unmarshal([]byte(block), &fm))
	assert.Equal(t, "原因: 新评论", fm["feedback_comment"])
}

func TestSetFrontmatterFieldsCommentWithColonRoundTripsYAML(t *testing.T) {
	content := "---\ntitle: Alpha\n---\n\nBody\n"
	got := setFrontmatterFields(content, map[string]string{
		"feedback_comment": yamlQuote("原因: 过度合并"),
	})
	block := extractFrontmatterBlock(got)

	var fm map[string]string
	require.NoError(t, yaml.Unmarshal([]byte(block), &fm))
	assert.Equal(t, "原因: 过度合并", fm["feedback_comment"])
}

func TestSetFrontmatterFieldsCreatesFrontmatterWhenMissing(t *testing.T) {
	content := "Body only\n"
	got := setFrontmatterFields(content, map[string]string{
		"feedback_comment": yamlQuote(""),
		"feedback_status":  "handled",
	})

	assert.Equal(t,
		"---\nfeedback_comment: \"\"\nfeedback_status: handled\n---\n\nBody only\n",
		got)
}
