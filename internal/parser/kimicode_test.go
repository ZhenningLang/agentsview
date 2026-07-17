package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeKimiCodeTestFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestDiscoverKimiCodeSessions(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "wd_my-app_aabbccddeeff")
	sessDir := filepath.Join(
		wsDir, "session_11111111-2222-3333-4444-555555555555")
	mainWire := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	subWire := filepath.Join(sessDir, "agents", "agent-0", "wire.jsonl")
	otherMain := filepath.Join(
		wsDir, "session_aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"agents", "main", "wire.jsonl")

	require.NoError(t, writeKimiCodeTestFile(mainWire, "{}\n"))
	require.NoError(t, writeKimiCodeTestFile(subWire, "{}\n"))
	require.NoError(t, writeKimiCodeTestFile(otherMain, "{}\n"))
	// A session dir without any wire.jsonl is skipped.
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(wsDir, "session_00000000-0000-0000-0000-000000000000",
			"logs", "kimi-code.log"), "log"))
	// Stray files at the wrong depth are skipped.
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(wsDir, "state.json"), "{}"))
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(root, "session_index.jsonl"), "{}"))

	files := DiscoverKimiCodeSessions(root)
	require.Len(t, files, 3)
	assert.Equal(t, subWire, files[0].Path)
	assert.Equal(t, mainWire, files[1].Path)
	assert.Equal(t, otherMain, files[2].Path)
	for _, f := range files {
		assert.Equal(t, "wd_my-app_aabbccddeeff", f.Project)
		assert.Equal(t, AgentKimiCode, f.Agent)
	}

	assert.Nil(t, DiscoverKimiCodeSessions(filepath.Join(root, "missing")))
	assert.Nil(t, DiscoverKimiCodeSessions(""))
}

func TestFindKimiCodeSourceFile(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "wd_my-app_aabbccddeeff")
	mainWire := filepath.Join(
		wsDir, "session_11111111-2222-3333-4444-555555555555",
		"agents", "main", "wire.jsonl")
	subWire := filepath.Join(
		wsDir, "session_11111111-2222-3333-4444-555555555555",
		"agents", "agent-0", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(mainWire, "{}\n"))
	require.NoError(t, writeKimiCodeTestFile(subWire, "{}\n"))

	assert.Equal(t, mainWire, FindKimiCodeSourceFile(
		root, "session_11111111-2222-3333-4444-555555555555"))
	assert.Equal(t, subWire, FindKimiCodeSourceFile(
		root, "session_11111111-2222-3333-4444-555555555555:agent-0"))
	assert.Empty(t, FindKimiCodeSourceFile(
		root, "session_99999999-9999-9999-9999-999999999999"))
	assert.Empty(t, FindKimiCodeSourceFile(root, "../bad"))
	assert.Empty(t, FindKimiCodeSourceFile(root, "session_x:../bad"))
	assert.Empty(t, FindKimiCodeSourceFile("", "session_11111111"))
}

const kimiCodeBasicWire = `{"type":"metadata","protocol_version":"1.4","created_at":1782441774650}
{"type":"config.update","profileName":"agent","systemPrompt":"You are Kimi Code CLI."}
{"type":"tools.set_active_tools","names":["Read","Bash"],"time":1782441774650}
{"type":"config.update","modelAlias":"kimi-code/kimi-for-coding","thinkingLevel":"high","time":1782441774651}
{"type":"permission.set_mode","mode":"auto","time":1782441774656}
{"type":"turn.prompt","input":[{"type":"text","text":"run echo"}],"origin":{"kind":"user"},"time":1782441774677}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"run echo"}],"toolCalls":[],"origin":{"kind":"user"}},"time":1782441774677}
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>auto mode</system-reminder>"}],"toolCalls":[],"origin":{"kind":"injection","variant":"permission_mode"}},"time":1782441774678}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1782441774678}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"s1","part":{"type":"think","think":"thinking..."}},"time":1782441777167}
{"type":"context.append_loop_event","event":{"type":"tool.call","uuid":"tool_1","turnId":"0","step":1,"stepUuid":"s1","toolCallId":"tool_1","name":"Bash","args":{"command":"echo HI"},"description":"Running: echo HI"},"time":1782441777188}
{"type":"context.append_loop_event","event":{"type":"tool.result","parentUuid":"tool_1","toolCallId":"tool_1","result":{"output":"HI\n"}},"time":1782441777195}
{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"usage":{"inputOther":1436,"output":47,"inputCacheRead":14592,"inputCacheCreation":0},"finishReason":"tool_use"},"time":1782441777195}
{"type":"usage.record","model":"kimi-code/kimi-for-coding","usage":{"inputOther":1436,"output":47,"inputCacheRead":14592,"inputCacheCreation":0},"usageScope":"turn","time":1782441777195}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s2","turnId":"0","step":2},"time":1782441777197}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p2","turnId":"0","step":2,"stepUuid":"s2","part":{"type":"text","text":"HI"}},"time":1782441779758}
{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s2","turnId":"0","step":2,"usage":{"inputOther":226,"output":33,"inputCacheRead":15872,"inputCacheCreation":0},"finishReason":"end_turn"},"time":1782441779758}
{"type":"usage.record","model":"kimi-code/kimi-for-coding","usage":{"inputOther":226,"output":33,"inputCacheRead":15872,"inputCacheCreation":0},"usageScope":"turn","time":1782441779758}
`

func TestParseKimiCodeSession_Basic(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(wirePath, kimiCodeBasicWire))
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(sessDir, "state.json"),
		`{"createdAt":"2026-06-26T02:42:54.628Z","updatedAt":"2026-06-26T02:42:54.673Z","title":"smoke test title","isCustomTitle":false,"agents":{"main":{"type":"main","parentAgentId":null}},"custom":{},"workDir":"/Users/alice/Projects/my-app","lastPrompt":"run echo"}`))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	sess := result.Session
	assert.Equal(t,
		"kimicode:session_11111111-2222-3333-4444-555555555555",
		sess.ID)
	assert.Equal(t, AgentKimiCode, sess.Agent)
	assert.Equal(t, "test-machine", sess.Machine)
	assert.Equal(t, "my-app", sess.Project)
	assert.Equal(t, "/Users/alice/Projects/my-app", sess.Cwd)
	assert.Equal(t, "smoke test title", sess.SessionName)
	assert.Equal(t, "run echo", sess.FirstMessage)
	assert.Equal(t, 5, sess.MessageCount)
	assert.Equal(t, 1, sess.UserMessageCount)
	assert.Equal(t,
		time.UnixMilli(1782441774650).UTC(), sess.StartedAt.UTC())
	assert.Equal(t,
		time.UnixMilli(1782441779758).UTC(), sess.EndedAt.UTC())
	assert.True(t, sess.HasTotalOutputTokens)
	assert.Equal(t, 80, sess.TotalOutputTokens)
	assert.True(t, sess.HasPeakContextTokens)
	assert.Equal(t, 16098, sess.PeakContextTokens)
	assert.Equal(t, TokenAggregateUsageEvents, sess.AggregateTokenSource)
	assert.Empty(t, sess.ParentSessionID)
	assert.Equal(t, RelNone, sess.RelationshipType)

	require.Len(t, result.Messages, 5)

	userMsg := result.Messages[0]
	assert.Equal(t, RoleUser, userMsg.Role)
	assert.Equal(t, "run echo", userMsg.Content)
	assert.False(t, userMsg.IsSystem)

	ctxCard := result.Messages[1]
	assert.True(t, ctxCard.IsSystem)
	assert.Equal(t, "system", ctxCard.SourceType)
	assert.Equal(t, "injection", ctxCard.SourceSubtype)
	assert.Contains(t, ctxCard.Content, "auto mode")

	asst1 := result.Messages[2]
	assert.Equal(t, RoleAssistant, asst1.Role)
	assert.True(t, asst1.HasThinking)
	assert.Equal(t, "thinking...", asst1.ThinkingText)
	assert.True(t, asst1.HasToolUse)
	require.Len(t, asst1.ToolCalls, 1)
	assert.Equal(t, "tool_1", asst1.ToolCalls[0].ToolUseID)
	assert.Equal(t, "Bash", asst1.ToolCalls[0].ToolName)
	assert.Equal(t, "Bash", asst1.ToolCalls[0].Category)
	assert.JSONEq(t, `{"command":"echo HI"}`,
		asst1.ToolCalls[0].InputJSON)
	assert.Equal(t, "tool_use", asst1.StopReason)

	toolMsg := result.Messages[3]
	assert.Equal(t, RoleUser, toolMsg.Role)
	require.Len(t, toolMsg.ToolResults, 1)
	assert.Equal(t, "tool_1", toolMsg.ToolResults[0].ToolUseID)
	assert.Equal(t, `"HI\n"`, toolMsg.ToolResults[0].ContentRaw)

	asst2 := result.Messages[4]
	assert.Equal(t, RoleAssistant, asst2.Role)
	assert.Equal(t, "HI", asst2.Content)
	assert.Equal(t, "end_turn", asst2.StopReason)

	require.Len(t, result.UsageEvents, 2)
	ev0 := result.UsageEvents[0]
	assert.Equal(t, sess.ID, ev0.SessionID)
	assert.Equal(t, "usage.record", ev0.Source)
	assert.Equal(t, "kimi-code/kimi-for-coding", ev0.Model)
	assert.Equal(t, 1436, ev0.InputTokens)
	assert.Equal(t, 47, ev0.OutputTokens)
	assert.Equal(t, 14592, ev0.CacheReadInputTokens)
	assert.Equal(t, 0, ev0.CacheCreationInputTokens)
	assert.Equal(t,
		time.UnixMilli(1782441777195).UTC().Format(time.RFC3339Nano),
		ev0.OccurredAt)
	assert.NotEmpty(t, ev0.DedupKey)
	assert.NotEqual(t, ev0.DedupKey, result.UsageEvents[1].DedupKey)
}

const kimiCodeMinimalWire = `{"type":"metadata","protocol_version":"1.4","created_at":1782441774650}
{"type":"turn.prompt","input":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"time":1782441774677}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1782441774678}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"s1","part":{"type":"text","text":"hello"}},"time":1782441774700}
{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"usage":{"inputOther":10,"output":5,"inputCacheRead":0,"inputCacheCreation":0},"finishReason":"end_turn"},"time":1782441774710}
{"type":"usage.record","model":"kimi-code/kimi-for-coding","usage":{"inputOther":10,"output":5,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":1782441774710}
`

func TestParseKimiCodeSession_Subagent(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "agent-0", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(wirePath, kimiCodeMinimalWire))
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(sessDir, "state.json"),
		`{"title":"parent session","workDir":"/Users/alice/Projects/my-app","forkedFrom":"session_00000000-0000-0000-0000-000000000000","agents":{"main":{"type":"main","parentAgentId":null},"agent-0":{"type":"sub","parentAgentId":"main"}}}`))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	sess := result.Session
	assert.Equal(t,
		"kimicode:session_11111111-2222-3333-4444-555555555555:agent-0",
		sess.ID)
	assert.Equal(t,
		"kimicode:session_11111111-2222-3333-4444-555555555555",
		sess.ParentSessionID)
	assert.Equal(t, RelSubagent, sess.RelationshipType)
	// forkedFrom applies to the main wire only, never to subagents.
}

func TestParseKimiCodeSession_Fork(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_22222222-3333-4444-5555-666666666666")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(wirePath, kimiCodeMinimalWire))
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(sessDir, "state.json"),
		`{"title":"Fork: some session","workDir":"/Users/alice/Projects/my-app","forkedFrom":"session_11111111-2222-3333-4444-555555555555","agents":{"main":{"type":"main","parentAgentId":null}}}`))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	sess := result.Session
	assert.Equal(t,
		"kimicode:session_22222222-3333-4444-5555-666666666666",
		sess.ID)
	assert.Equal(t,
		"kimicode:session_11111111-2222-3333-4444-555555555555",
		sess.ParentSessionID)
	assert.Equal(t, RelFork, sess.RelationshipType)
}

func TestParseKimiCodeSession_NoStateJSON(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(wirePath, kimiCodeMinimalWire))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	sess := result.Session
	assert.Equal(t, "my-app", sess.Project)
	assert.Empty(t, sess.SessionName)
	assert.Empty(t, sess.Cwd)
	assert.Empty(t, sess.ParentSessionID)
}

func TestParseKimiCodeSession_MalformedAndUnknown(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	wire := `{"type":"metadata","protocol_version":"9.9","created_at":1782441774650}
not json at all
{"type":"llm.request","foo":"bar","time":1782441774660}
{"type":"turn.prompt","input":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"time":1782441774677}
{"type":"context.append_loop_event","event":{"type":"future.event","x":1},"time":1782441774680}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1782441774681}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"s1","part":{"type":"text","text":"ok"}},"time":1782441774690}
`
	require.NoError(t, writeKimiCodeTestFile(wirePath, wire))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Session.MalformedLines)
	require.Len(t, result.Messages, 2)
	assert.Equal(t, RoleUser, result.Messages[0].Role)
	assert.Equal(t, RoleAssistant, result.Messages[1].Role)
	assert.Empty(t, result.UsageEvents)
	assert.False(t, result.Session.HasTotalOutputTokens)
}

func TestParseKimiCodeSession_EmptyWireIgnored(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(wirePath,
		`{"type":"metadata","protocol_version":"1.4","created_at":1782441774650}`+"\n"))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseKimiCodeSession_StepEndUsageFallback(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	// No usage.record lines; usage only on step.end.
	wire := `{"type":"metadata","protocol_version":"1.4","created_at":1782441774650}
{"type":"turn.prompt","input":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"time":1782441774677}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1782441774678}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"s1","part":{"type":"text","text":"ok"}},"time":1782441774690}
{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"usage":{"inputOther":10,"output":5,"inputCacheRead":7,"inputCacheCreation":3},"finishReason":"end_turn"},"time":1782441774700}
`
	require.NoError(t, writeKimiCodeTestFile(wirePath, wire))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.UsageEvents, 1)
	assert.Equal(t, "step.end", result.UsageEvents[0].Source)
	assert.Equal(t, 5, result.UsageEvents[0].OutputTokens)
	assert.True(t, result.Session.HasTotalOutputTokens)
	assert.Equal(t, 5, result.Session.TotalOutputTokens)
	assert.True(t, result.Session.HasPeakContextTokens)
	assert.Equal(t, 20, result.Session.PeakContextTokens)
	assert.Equal(t, TokenAggregateUsageEvents,
		result.Session.AggregateTokenSource)
}

func TestParseKimiCodeSession_SteerContextCard(t *testing.T) {
	root := t.TempDir()
	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	wire := `{"type":"metadata","protocol_version":"1.4","created_at":1782441774650}
{"type":"turn.prompt","input":[{"type":"text","text":"start"}],"origin":{"kind":"user"},"time":1782441774677}
{"type":"turn.steer","input":[{"type":"text","text":"<notification>task failed</notification>"}],"origin":{"kind":"background_task","taskId":"bash-x","status":"failed"},"time":1782441774700}
{"type":"turn.steer","input":[{"type":"text","text":"actually do Y instead"}],"origin":{"kind":"user"},"time":1782441774750}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1782441774760}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"s1","part":{"type":"text","text":"done"}},"time":1782441774770}
`
	require.NoError(t, writeKimiCodeTestFile(wirePath, wire))

	result, err := ParseKimiCodeSession(
		wirePath, "wd_my-app_aabbccddeeff", "test-machine")
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.Messages, 4)
	assert.Equal(t, "start", result.Messages[0].Content)
	card := result.Messages[1]
	assert.True(t, card.IsSystem)
	assert.Equal(t, "background_task", card.SourceSubtype)
	assert.Contains(t, card.Content, "task failed")
	steer := result.Messages[2]
	assert.Equal(t, RoleUser, steer.Role)
	assert.False(t, steer.IsSystem)
	assert.Equal(t, "actually do Y instead", steer.Content)
	assert.Equal(t, 2, result.Session.UserMessageCount)
}
