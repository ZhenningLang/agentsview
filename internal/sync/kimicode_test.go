package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/parser"
)

func TestEngineClassifyKimiCodePaths(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	engine := NewEngine(db, EngineConfig{
		AgentDirs: map[parser.AgentType][]string{
			parser.AgentKimiCode: {root},
		},
		Machine: "local",
	})

	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	mainWire := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	subWire := filepath.Join(sessDir, "agents", "agent-0", "wire.jsonl")
	stateFile := filepath.Join(sessDir, "state.json")
	logFile := filepath.Join(sessDir, "logs", "kimi-code.log")
	strayWire := filepath.Join(root, "wire.jsonl")
	for _, path := range []string{
		mainWire, subWire, stateFile, logFile, strayWire,
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o644))
	}

	got, ok := engine.classifyOnePath(mainWire, nil)
	require.True(t, ok, "main wire path did not classify")
	assert.Equal(t, mainWire, got.Path)
	assert.Equal(t, "wd_my-app_aabbccddeeff", got.Project)
	assert.Equal(t, parser.AgentKimiCode, got.Agent)

	got, ok = engine.classifyOnePath(subWire, nil)
	require.True(t, ok, "subagent wire path did not classify")
	assert.Equal(t, subWire, got.Path)
	assert.Equal(t, parser.AgentKimiCode, got.Agent)

	for _, path := range []string{stateFile, logFile, strayWire} {
		got, ok = engine.classifyOnePath(path, nil)
		assert.False(t, ok, "%s should not classify, got %+v", path, got)
	}
}

func TestEngineProcessKimiCode(t *testing.T) {
	db := openTestDB(t)
	root := t.TempDir()
	engine := NewEngine(db, EngineConfig{
		AgentDirs: map[parser.AgentType][]string{
			parser.AgentKimiCode: {root},
		},
		Machine: "local",
	})

	sessDir := filepath.Join(
		root, "wd_my-app_aabbccddeeff",
		"session_11111111-2222-3333-4444-555555555555")
	wirePath := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	wire := `{"type":"metadata","protocol_version":"1.4","created_at":1782441774650}
{"type":"turn.prompt","input":[{"type":"text","text":"hi"}],"origin":{"kind":"user"},"time":1782441774677}
{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"s1","turnId":"0","step":1},"time":1782441774678}
{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"p1","turnId":"0","step":1,"stepUuid":"s1","part":{"type":"text","text":"hello"}},"time":1782441774700}
{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"s1","turnId":"0","step":1,"usage":{"inputOther":10,"output":5,"inputCacheRead":0,"inputCacheCreation":0},"finishReason":"end_turn"},"time":1782441774710}
{"type":"usage.record","model":"kimi-code/kimi-for-coding","usage":{"inputOther":10,"output":5,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":1782441774710}
`
	require.NoError(t, os.MkdirAll(filepath.Dir(wirePath), 0o755))
	require.NoError(t, os.WriteFile(wirePath, []byte(wire), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(sessDir, "state.json"),
		[]byte(`{"title":"t","workDir":"/Users/alice/Projects/my-app"}`),
		0o644))

	info, err := os.Stat(wirePath)
	require.NoError(t, err)

	res := engine.processKimiCode(parser.DiscoveredFile{
		Path:    wirePath,
		Project: "wd_my-app_aabbccddeeff",
		Agent:   parser.AgentKimiCode,
	}, info)
	require.False(t, res.skip, "result skipped")
	require.NoError(t, res.err)
	require.Len(t, res.results, 1)

	sess := res.results[0].Session
	assert.Equal(t,
		"kimicode:session_11111111-2222-3333-4444-555555555555",
		sess.ID)
	assert.Equal(t, "my-app", sess.Project)
	assert.Equal(t, "local", sess.Machine)
	assert.NotEmpty(t, sess.File.Hash)
	assert.Equal(t, 5, sess.TotalOutputTokens)
	require.Len(t, res.results[0].UsageEvents, 1)
}
