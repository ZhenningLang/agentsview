package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.kenn.io/agentsview/internal/testjsonl"
)

func writeSkillFixture(t *testing.T, root, dir, name string) string {
	t.Helper()
	path := filepath.Join(root, dir, "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("---\nname: "+name+"\n---\nbody\n"), 0o644))
	return path
}

func TestParseCodexSession_SkillNameFromSkillRead(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, filepath.Join("skills", "reviewer"), "reviewer")
	content := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON("skill-full", root, "user", tsEarly),
		testjsonl.CodexMsgJSON("user", "read skill", tsEarlyS1),
		testjsonl.CodexFunctionCallArgsJSON("exec_command", map[string]any{
			"cmd": "cat skills/reviewer/SKILL.md",
		}, tsEarlyS5),
	)
	_, msgs := runCodexParserTest(t, "codex-skill.jsonl", content, false)
	require.Len(t, msgs, 2)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "reviewer", msgs[1].ToolCalls[0].SkillName)
}

func TestInferCodexSkillNameReadCommandClassification(t *testing.T) {
	root := t.TempDir()
	path := writeSkillFixture(t, root, filepath.Join("skills", "shell reader"), "shell-reader")
	relPath, err := filepath.Rel(root, path)
	require.NoError(t, err)

	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{"cat relative", "cat '" + relPath + "'", "shell-reader"},
		{"grep file operand", "rg --glob '*.md' name '" + relPath + "'", "shell-reader"},
		{"grep pattern file ignored", "grep -f '" + relPath + "' ignored.txt", ""},
		{"grep pattern file keeps later file operand", "grep -f patterns.txt '" + relPath + "'", "shell-reader"},
		{"grep equals pattern file keeps later file operand", "grep --file=patterns.txt '" + relPath + "'", "shell-reader"},
		{"rg pattern file keeps later file operand", "rg -f patterns.txt '" + relPath + "'", "shell-reader"},
		{"rg pattern before file", "rg name '" + relPath + "'", "shell-reader"},
		{"multi segment", "printf hi && head -n 5 '" + relPath + "'", "shell-reader"},
		{"write verb ignored", "cp " + relPath + " /tmp/SKILL.md", ""},
		{"python verb ignored", "python '" + relPath + "'", ""},
		{"redirect target ignored", "printf hi > " + relPath, ""},
		{"glob ignored", "cat " + filepath.Join("skills", "*", "SKILL.md"), ""},
		{"sed inplace ignored", "sed -i s/a/b/ " + relPath, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]any{
				"cmd":     tt.cmd,
				"workdir": root,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, inferCodexSkillNameWithBase("exec_command", string(input), ""))
		})
	}
}

func TestSkillNameFromPathFrontmatterAndSafety(t *testing.T) {
	root := t.TempDir()
	frontmatterPath := writeSkillFixture(t, root, filepath.Join("skills", "frontmatter"), "front-name")
	homeSkillPath := writeSkillFixture(t, root, ".", "home-skill")
	missingPath := filepath.Join(root, "skills", "fallback", "SKILL.md")
	barePath := "SKILL.md"
	tooLargePath := filepath.Join(root, "skills", "large", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(tooLargePath), 0o755))
	require.NoError(t, os.WriteFile(tooLargePath,
		[]byte("---\n"+strings.Repeat("#", maxSkillFrontmatterSize)+"\nname: too-large\n---\n"), 0o644))

	assert.Equal(t, "front-name", skillNameFromPath(frontmatterPath, ""))
	assert.Equal(t, "fallback", skillNameFromPath(missingPath, ""))
	assert.Empty(t, skillNameFromPath(barePath, ""))
	assert.Empty(t, skillNameFromPath(filepath.Join(root, "skills", "*", "SKILL.md"), ""))
	t.Setenv("HOME", filepath.Dir(homeSkillPath))
	assert.Equal(t, "home-skill", skillNameFromPath("~/SKILL.md", ""))
	assert.Equal(t, "win-skill", skillNameFromPath(`C:\Users\me\skills\win-skill\SKILL.md`, ""),
		"Windows-style path separators should be understood as skill paths")
	assert.Equal(t, "large", skillNameFromPath(tooLargePath, ""),
		"frontmatter past the 64KiB read bound should fall back to parent directory")

	if runtime.GOOS != "windows" {
		symlinkPath := filepath.Join(root, "skills", "link", "SKILL.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o755))
		require.NoError(t, os.Symlink(frontmatterPath, symlinkPath))
		assert.Equal(t, "link", skillNameFromPath(symlinkPath, ""),
			"symlink frontmatter must not be followed; fallback to directory is allowed")
	}
}

func TestInferSkillNameFromJSONPathsUsesDeterministicPathKeys(t *testing.T) {
	root := t.TempDir()
	preferred := writeSkillFixture(t, root, filepath.Join("skills", "preferred"), "preferred")
	other := writeSkillFixture(t, root, filepath.Join("skills", "other"), "other")
	input, err := json.Marshal(map[string]any{
		"zzz":  other,
		"path": preferred,
	})
	require.NoError(t, err)

	for range 20 {
		assert.Equal(t, "preferred", inferSkillNameFromJSONPaths(string(input), ""))
	}
}

func TestParseCodexSessionFrom_SkillNameUsesSeededCWD(t *testing.T) {
	root := t.TempDir()
	writeSkillFixture(t, root, filepath.Join("skills", "incremental"), "incremental")
	initial := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON("skill-inc", root, "user", tsEarly),
		testjsonl.CodexMsgJSON("user", "hello", tsEarlyS1),
	)
	path := createTestFile(t, "codex-skill-inc.jsonl", initial)
	info, err := os.Stat(path)
	require.NoError(t, err)
	offset := info.Size()
	appendTestFile(t, path, testjsonl.CodexFunctionCallArgsJSON("exec_command", map[string]any{
		"cmd": "cat skills/incremental/SKILL.md",
	}, tsLate))

	msgs, _, _, err := ParseCodexSessionFrom(path, offset, 1, false)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].ToolCalls, 1)
	assert.Equal(t, "incremental", msgs[0].ToolCalls[0].SkillName)
}

func TestParseCodexSession_SkillNameUsesToolWorkdirOverSessionCWD(t *testing.T) {
	sessionRoot := t.TempDir()
	toolRoot := t.TempDir()
	writeSkillFixture(t, sessionRoot, filepath.Join("skills", "wrong"), "wrong")
	writeSkillFixture(t, toolRoot, filepath.Join("skills", "tool"), "tool")
	content := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON("skill-workdir", sessionRoot, "user", tsEarly),
		testjsonl.CodexMsgJSON("user", "read skill", tsEarlyS1),
		testjsonl.CodexFunctionCallArgsJSON("exec_command", map[string]any{
			"cmd":     "cat skills/tool/SKILL.md",
			"workdir": toolRoot,
		}, tsEarlyS5),
	)
	_, msgs := runCodexParserTest(t, "codex-skill-workdir.jsonl", content, false)
	require.Len(t, msgs, 2)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "tool", msgs[1].ToolCalls[0].SkillName)
}

func appendTestFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestExtractTextContent_SkillNameInferenceAndExplicitPrecedence(t *testing.T) {
	root := t.TempDir()
	readPath := writeSkillFixture(t, root, filepath.Join("skills", "reader"), "reader")
	input := map[string]any{"path": readPath}
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	content := gjson.Parse(`[
		{"type":"tool_use","id":"r1","name":"Read","input":` + string(raw) + `},
		{"type":"tool_use","id":"w1","name":"Write","input":{"path":"` + readPath + `"}},
		{"type":"tool_use","id":"s1","name":"skill","input":{"name":"explicit"}}
	]`)
	_, _, _, _, calls, _ := ExtractTextContent(content)
	require.Len(t, calls, 3)
	assert.Equal(t, "reader", calls[0].SkillName)
	assert.Empty(t, calls[1].SkillName)
	assert.Equal(t, "explicit", calls[2].SkillName)
}

func TestCursorPlainTextToolInputJSONAndSkillName(t *testing.T) {
	root := t.TempDir()
	path := writeSkillFixture(t, root, filepath.Join("skills", "cursor"), "cursor")
	lines := []string{
		"[Tool call] ReadFile",
		"  path=" + path,
	}
	_, _, calls := extractAssistantContent(lines)
	require.Len(t, calls, 1)
	assert.JSONEq(t, `{"path":"`+path+`"}`, calls[0].InputJSON)
	assert.Equal(t, "cursor", calls[0].SkillName)
}

func TestCursorApplyPatchDoesNotInferSkillName(t *testing.T) {
	lines := []string{
		"[Tool call] ApplyPatch",
		"  *** Begin Patch",
		"  *** Update File: skills/reviewer/SKILL.md",
		"  *** End Patch",
	}
	_, _, calls := extractAssistantContent(lines)
	require.Len(t, calls, 1)
	assert.NotEmpty(t, calls[0].InputJSON)
	assert.Empty(t, calls[0].SkillName)
}
