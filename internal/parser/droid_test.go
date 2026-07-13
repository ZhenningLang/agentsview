package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDroidTestFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestDiscoverDroidSessions(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "-Users-alice-Projects-my-app")
	require.NoError(t, writeDroidTestFile(filepath.Join(projectDir, "ses_1.jsonl"), `{"type":"session_start","id":"ses_1"}`))
	require.NoError(t, writeDroidTestFile(filepath.Join(projectDir, "ses_1.settings.json"), `{}`))
	require.NoError(t, writeDroidTestFile(filepath.Join(projectDir, "notes.txt"), `ignore`))

	files := DiscoverDroidSessions(root)
	require.Len(t, files, 1)
	assert.Equal(t, filepath.Join(projectDir, "ses_1.jsonl"), files[0].Path)
	assert.Equal(t, "my_app", files[0].Project)
	assert.Equal(t, AgentDroid, files[0].Agent)

	assert.Nil(t, DiscoverDroidSessions(filepath.Join(root, "missing")))
}

func TestFindDroidSourceFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "-Users-alice-Projects-my-app", "ses_1.jsonl")
	require.NoError(t, writeDroidTestFile(path, `{"type":"session_start","id":"ses_1"}`))

	assert.Equal(t, path, FindDroidSourceFile(root, "ses_1"))
	assert.Empty(t, FindDroidSourceFile(root, "../bad"))
	assert.Empty(t, FindDroidSourceFile(root, "missing"))
}

func TestParseDroidSession_IncludesSettingsUsageEvent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "-Users-alice-Projects-my-app", "ses_1.jsonl")
	require.NoError(t, writeDroidTestFile(path, `{"type":"session_start","id":"ses_1","title":"Build droid stats","sessionTitle":"Droid Stats","version":2,"cwd":"/Users/alice/Projects/my-app"}
{"type":"message","id":"msg_user","timestamp":"2026-04-08T07:25:24.897Z","message":{"role":"user","content":[{"type":"text","text":"count droid usage"}]}}
{"type":"message","id":"msg_assistant","timestamp":"2026-04-08T07:26:24.897Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"plan"},{"type":"text","text":"done"},{"type":"tool_use","id":"tool_1","name":"Read","input":{"file_path":"README.md"}}]}}
{"type":"message","id":"msg_tool","timestamp":"2026-04-08T07:27:24.897Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","is_error":false,"content":"file content"}]}}
`))
	require.NoError(t, writeDroidTestFile(filepath.Join(root, "-Users-alice-Projects-my-app", "ses_1.settings.json"), `{
  "model": "custom:GPT-5.4-1",
  "tokenUsage": {
    "inputTokens": 1000,
    "outputTokens": 200,
    "cacheCreationTokens": 30,
    "cacheReadTokens": 400,
    "thinkingTokens": 50
  }
}`))

	result, err := ParseDroidSession(path, "my_app", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	sess := result.Session
	assert.Equal(t, "droid:ses_1", sess.ID)
	assert.Equal(t, AgentDroid, sess.Agent)
	assert.Equal(t, "my_app", sess.Project)
	assert.Equal(t, "/Users/alice/Projects/my-app", sess.Cwd)
	assert.Equal(t, "Droid Stats", sess.SessionName)
	assert.Equal(t, "count droid usage", sess.FirstMessage)
	assert.Equal(t, 4, sess.MessageCount)
	assert.Equal(t, 2, sess.UserMessageCount)
	assert.True(t, sess.HasTotalOutputTokens)
	assert.Equal(t, 200, sess.TotalOutputTokens)
	assert.True(t, sess.HasPeakContextTokens)
	assert.Equal(t, 1430, sess.PeakContextTokens)
	assert.Equal(t, TokenAggregateSummary, sess.AggregateTokenSource)

	require.Len(t, result.Messages, 4)
	assert.True(t, result.Messages[0].IsSystem)
	assert.Equal(t, "system", result.Messages[0].SourceType)
	assert.Equal(t, "session_start", result.Messages[0].SourceSubtype)
	assert.Contains(t, result.Messages[0].Content, "cwd: /Users/alice/Projects/my-app")
	assert.Equal(t, RoleUser, result.Messages[1].Role)
	assert.Equal(t, RoleAssistant, result.Messages[2].Role)
	assert.True(t, result.Messages[2].HasThinking)
	assert.True(t, result.Messages[2].HasToolUse)
	require.Len(t, result.Messages[2].ToolCalls, 1)
	assert.Equal(t, "Read", result.Messages[2].ToolCalls[0].ToolName)
	require.Len(t, result.Messages[3].ToolResults, 1)
	assert.Equal(t, "tool_1", result.Messages[3].ToolResults[0].ToolUseID)

	require.Len(t, result.UsageEvents, 1)
	usage := result.UsageEvents[0]
	assert.Equal(t, "droid:ses_1", usage.SessionID)
	assert.Equal(t, "droid-settings", usage.Source)
	assert.Equal(t, "custom:GPT-5.4-1", usage.Model)
	assert.Equal(t, 1000, usage.InputTokens)
	assert.Equal(t, 200, usage.OutputTokens)
	assert.Equal(t, 30, usage.CacheCreationInputTokens)
	assert.Equal(t, 400, usage.CacheReadInputTokens)
	assert.Equal(t, 50, usage.ReasoningTokens)
	assert.Equal(t, "session:droid:ses_1", usage.DedupKey)
}

func TestParseDroidSession_NoSettingsUsage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "-Users-alice-Projects-my-app", "ses_1.jsonl")
	require.NoError(t, writeDroidTestFile(path, `{"type":"session_start","id":"ses_1","cwd":"/Users/alice/Projects/my-app"}
{"type":"message","id":"msg_user","timestamp":"2026-04-08T07:25:24.897Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
`))

	result, err := ParseDroidSession(path, "my_app", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.UsageEvents)
	assert.False(t, result.Session.HasTotalOutputTokens)
	assert.False(t, result.Session.HasPeakContextTokens)
}

func TestParseDroidSession_SessionStartOnlyIsIgnored(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "-Users-alice-Projects-my-app", "ses_1.jsonl")
	require.NoError(t, writeDroidTestFile(path, `{"type":"session_start","id":"ses_1","cwd":"/Users/alice/Projects/my-app"}`))

	result, err := ParseDroidSession(path, "my_app", "test-machine")
	require.NoError(t, err)
	assert.Nil(t, result)
}
