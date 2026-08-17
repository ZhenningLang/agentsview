package server_test

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

func canonicalTestPath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = filepath.Clean(resolved)
	}
	if runtime.GOOS == "darwin" && strings.HasPrefix(clean, "/private/") {
		publicPath := filepath.Clean(strings.TrimPrefix(clean, "/private"))
		if info, err := os.Stat(publicPath); err == nil && info.IsDir() {
			return publicPath
		}
	}
	return clean
}

func assertSamePath(t *testing.T, label, got, want string) {
	t.Helper()
	got = canonicalTestPath(got)
	want = canonicalTestPath(want)
	if got == want {
		return
	}
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(want)
	if gotErr == nil && wantErr == nil && os.SameFile(gotInfo, wantInfo) {
		return
	}
	assert.Fail(t, "path mismatch", "%s = %q, want %q", label, got, want)
}

func phase18PromptGlob(t *testing.T, sessionID string, ordinal int) string {
	t.Helper()
	cacheDir, err := os.UserCacheDir()
	require.NoError(t, err)
	return filepath.Join(
		cacheDir,
		"agentsview",
		"claude-message-points",
		fmt.Sprintf("%s-ordinal-%d-*.txt", sessionID, ordinal),
	)
}

func phase18RemovePrompts(t *testing.T, sessionID string, ordinal int) {
	t.Helper()
	matches, err := filepath.Glob(phase18PromptGlob(t, sessionID, ordinal))
	require.NoError(t, err)
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

func phase18SinglePrompt(t *testing.T, sessionID string, ordinal int) string {
	t.Helper()
	matches, err := filepath.Glob(phase18PromptGlob(t, sessionID, ordinal))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	return matches[0]
}

func phase18AssertNoPrompts(t *testing.T, sessionID string, ordinal int) {
	t.Helper()
	matches, err := filepath.Glob(phase18PromptGlob(t, sessionID, ordinal))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func phase18AssertCommandConsumesPrompt(
	t *testing.T, command string, promptPath string,
) {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := phase18DecodePowerShell(t, command)
		quoted := "'" + strings.ReplaceAll(promptPath, "'", "''") + "'"
		assert.Contains(t, script,
			"Get-Content -Raw -Encoding UTF8 -LiteralPath "+quoted)
		assert.Contains(t, script,
			"Remove-Item -LiteralPath "+quoted+
				" -Force -ErrorAction SilentlyContinue")
		return
	}
	assert.Contains(t, command, "claude <")
	assert.Contains(t, command, "rm -f --")
	assert.Contains(t, command, promptPath)
}

func phase18DecodePowerShell(t *testing.T, command string) string {
	t.Helper()
	const prefix = "powershell.exe -NoProfile -EncodedCommand "
	require.True(t, strings.HasPrefix(command, prefix), "command = %q", command)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(command, prefix))
	require.NoError(t, err)
	require.Zero(t, len(raw)%2, "UTF-16LE byte length must be even")
	codeUnits := make([]uint16, len(raw)/2)
	for i := range codeUnits {
		codeUnits[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(codeUnits))
}

func TestResumeSession(t *testing.T) {
	te := setup(t)

	// Seed a claude session with an absolute project path.
	projectDir := t.TempDir()
	te.seedSession(t, "sess-1", projectDir, 5, func(s *db.Session) {
		s.Agent = "claude"
	})

	t.Run("command only", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/sessions/sess-1/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
			Cwd      string `json:"cwd"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		assert.NotEmpty(t, resp.Command)
		assertSamePath(t, "cwd", resp.Cwd, projectDir)
	})

	t.Run("not found", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/sessions/nonexistent/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("copilot command only", func(t *testing.T) {
		projectDir := t.TempDir()
		// Use a prefixed ID to exercise the agent-prefix stripping
		// logic (e.g. "copilot:abc123" → raw ID "abc123").
		te.seedSession(t, "copilot:abc123", projectDir, 3, func(s *db.Session) {
			s.Agent = "copilot"
		})
		w := te.post(t,
			"/api/v1/sessions/copilot:abc123/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		assert.Equal(t, "copilot --resume=abc123", resp.Command)
	})

	t.Run("kiro current-store command only", func(t *testing.T) {
		projectDir := t.TempDir()
		te.seedSession(t, "kiro:sqlite-chat", "kiro_app", 3, func(s *db.Session) {
			s.Agent = "kiro"
			s.Cwd = projectDir
		})
		w := te.post(t,
			"/api/v1/sessions/kiro:sqlite-chat/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
			Cwd      string `json:"cwd"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		const cmdSuffix = "' && kiro-cli chat --resume-id sqlite-chat"
		if !strings.HasPrefix(resp.Command, "cd '") ||
			!strings.HasSuffix(resp.Command, cmdSuffix) {
			assert.Fail(t, "command shape mismatch",
				"command = %q, want cd command ending with %q",
				resp.Command, cmdSuffix)
		} else {
			commandCwd := strings.TrimSuffix(
				strings.TrimPrefix(resp.Command, "cd '"),
				cmdSuffix,
			)
			assertSamePath(t, "command cwd", commandCwd, projectDir)
		}
		assertSamePath(t, "cwd", resp.Cwd, projectDir)
	})

	t.Run("kilo command only", func(t *testing.T) {
		projectDir := t.TempDir()
		te.seedSession(t, "kilo:ses_kilo", projectDir, 3, func(s *db.Session) {
			s.Agent = "kilo"
		})
		w := te.post(t,
			"/api/v1/sessions/kilo:ses_kilo/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
			Cwd      string `json:"cwd"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		assert.Equal(t, "kilo --session ses_kilo", resp.Command)
		assertSamePath(t, "cwd", resp.Cwd, projectDir)
	})

	t.Run("kilo command only quotes raw id", func(t *testing.T) {
		id := "kilo:$(whoami)"
		te.seedSession(t, id, t.TempDir(), 3, func(s *db.Session) {
			s.Agent = "kilo"
		})
		w := te.post(t,
			"/api/v1/sessions/"+url.PathEscape(id)+"/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		assert.Equal(t, "kilo --session '$(whoami)'", resp.Command)
	})

	t.Run("claude desktop rejects non-claude agent", func(t *testing.T) {
		te.seedSession(t, "codex-desk", t.TempDir(), 3, func(s *db.Session) {
			s.Agent = "codex"
		})
		w := te.post(t,
			"/api/v1/sessions/codex-desk/resume",
			`{"opener_id":"claude-desktop"}`,
		)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("cursor command only", func(t *testing.T) {
		projectDir := t.TempDir()
		runDir := filepath.Join(projectDir, "frontend")
		require.NoError(t, os.MkdirAll(runDir, 0o755))
		runDirJSON, _ := json.Marshal(runDir)
		sessionFile := filepath.Join(t.TempDir(), "cursor.jsonl")
		content := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Shell","input":{"command":"pwd","working_directory":` +
			string(runDirJSON) + `}}]}}` + "\n"
		require.NoError(t, os.WriteFile(sessionFile, []byte(content), 0o644))
		te.seedSession(t, "cursor:chat-1", projectDir, 3, func(s *db.Session) {
			s.Agent = "cursor"
			s.FilePath = &sessionFile
		})
		w := te.post(t,
			"/api/v1/sessions/cursor:chat-1/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
			Cwd      string `json:"cwd"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		wantProjectDir := canonicalTestPath(projectDir)
		assert.Equal(t,
			"cursor agent --resume chat-1 --workspace '"+wantProjectDir+"'",
			resp.Command)
		assertSamePath(t, "cwd", resp.Cwd, runDir)
	})

	t.Run("cursor command only falls back workspace to cwd", func(t *testing.T) {
		runDir := filepath.Join(t.TempDir(), "frontend")
		require.NoError(t, os.MkdirAll(runDir, 0o755))
		runDirJSON, _ := json.Marshal(runDir)
		sessionFile := filepath.Join(t.TempDir(), "cursor.jsonl")
		content := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Shell","input":{"command":"pwd","working_directory":` +
			string(runDirJSON) + `}}]}}` + "\n"
		require.NoError(t, os.WriteFile(sessionFile, []byte(content), 0o644))
		te.seedSession(t, "cursor:chat-2", "li_tools", 3, func(s *db.Session) {
			s.Agent = "cursor"
			s.FilePath = &sessionFile
		})
		w := te.post(t,
			"/api/v1/sessions/cursor:chat-2/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Launched bool   `json:"launched"`
			Command  string `json:"command"`
			Cwd      string `json:"cwd"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Launched, "expected launched=false for command_only")
		wantRunDir := canonicalTestPath(runDir)
		assert.Equal(t,
			"cursor agent --resume chat-2 --workspace '"+wantRunDir+"'",
			resp.Command)
		assertSamePath(t, "cwd", resp.Cwd, runDir)
	})

	t.Run("unsupported agent", func(t *testing.T) {
		te.seedSession(t, "vscode-1", "/tmp", 3, func(s *db.Session) {
			s.Agent = "vscode-copilot"
		})
		w := te.post(t,
			"/api/v1/sessions/vscode-1/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("deleted session rejected", func(t *testing.T) {
		te.seedSession(t, "del-1", "/tmp", 3, func(s *db.Session) {
			s.Agent = "claude"
		})
		require.NoError(t, te.db.SoftDeleteSession("del-1"))
		w := te.post(t,
			"/api/v1/sessions/del-1/resume",
			`{"command_only":true}`,
		)
		assertStatus(t, w, http.StatusNotFound)
	})
}

func TestPhase18MessagePointForkCommandOnly(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("USERPROFILE", cacheRoot)
	projectDir := t.TempDir()
	te := setup(t)
	te.seedSession(t, "phase18-command", projectDir, 4, func(s *db.Session) {
		s.Agent = "claude"
	})
	te.seedMessages(t, "phase18-command", 4, func(i int, m *db.Message) {
		switch i {
		case 0:
			m.Ordinal = 0
			m.Role = "user"
			m.Content = "phase18 first"
		case 1:
			m.Ordinal = 1
			m.Role = "assistant"
			m.Content = "phase18 second"
		case 2:
			m.Ordinal = 3
			m.Role = "assistant"
			m.Content = "phase18 selected sparse"
		case 3:
			m.Ordinal = 7
			m.Content = "phase18 after cut"
		}
	})

	w := te.post(t,
		"/api/v1/sessions/phase18-command/resume",
		`{"command_only":true,"from_ordinal":3,"fork_session":true}`,
	)
	assertStatus(t, w, http.StatusOK)
	var resp struct {
		Launched bool   `json:"launched"`
		Command  string `json:"command"`
		Cwd      string `json:"cwd"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Launched)
	assertSamePath(t, "cwd", resp.Cwd, projectDir)
	promptPath := phase18SinglePrompt(t, "phase18-command", 3)
	t.Cleanup(func() { _ = os.Remove(promptPath) })
	phase18AssertCommandConsumesPrompt(t, resp.Command, promptPath)

	data, err := os.ReadFile(promptPath)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "phase18 first")
	assert.Contains(t, text, "phase18 second")
	assert.Contains(t, text, "phase18 selected sparse")
	assert.NotContains(t, text, "phase18 after cut")
}

func TestPhase18MessagePointForkValidation(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("USERPROFILE", cacheRoot)
	projectDir := t.TempDir()

	tests := []struct {
		name       string
		id         string
		agent      string
		body       string
		status     int
		ordinal    int
		delete     bool
		remotePath string
	}{
		{
			name:    "non claude rejected",
			id:      "phase18-codex",
			agent:   "codex",
			body:    `{"command_only":true,"from_ordinal":0,"fork_session":true}`,
			status:  http.StatusBadRequest,
			ordinal: 0,
		},
		{
			name:    "negative ordinal rejected",
			id:      "phase18-negative",
			agent:   "claude",
			body:    `{"command_only":true,"from_ordinal":-1,"fork_session":true}`,
			status:  http.StatusBadRequest,
			ordinal: -1,
		},
		{
			name:    "missing ordinal rejected",
			id:      "phase18-missing",
			agent:   "claude",
			body:    `{"command_only":true,"from_ordinal":9,"fork_session":true}`,
			status:  http.StatusNotFound,
			ordinal: 9,
		},
		{
			name:    "fork required",
			id:      "phase18-fork-required",
			agent:   "claude",
			body:    `{"command_only":true,"from_ordinal":0}`,
			status:  http.StatusBadRequest,
			ordinal: 0,
		},
		{
			name:    "opener rejected",
			id:      "phase18-opener",
			agent:   "claude",
			body:    `{"command_only":true,"from_ordinal":0,"fork_session":true,"opener_id":"claude-desktop"}`,
			status:  http.StatusBadRequest,
			ordinal: 0,
		},
		{
			name:    "deleted session rejected",
			id:      "phase18-deleted",
			agent:   "claude",
			body:    `{"command_only":true,"from_ordinal":0,"fork_session":true}`,
			status:  http.StatusNotFound,
			ordinal: 0,
			delete:  true,
		},
		{
			name:       "remote id rejected",
			id:         "phase18-remote",
			agent:      "claude",
			body:       `{"command_only":true,"from_ordinal":0,"fork_session":true}`,
			status:     http.StatusBadRequest,
			ordinal:    0,
			remotePath: "host-a~phase18-remote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := setup(t)
			phase18RemovePrompts(t, tt.id, tt.ordinal)
			te.seedSession(t, tt.id, projectDir, 2, func(s *db.Session) {
				s.Agent = tt.agent
			})
			te.seedMessages(t, tt.id, 2)
			if tt.delete {
				require.NoError(t, te.db.SoftDeleteSession(tt.id))
			}
			pathID := tt.id
			if tt.remotePath != "" {
				pathID = tt.remotePath
			}
			w := te.post(t,
				"/api/v1/sessions/"+url.PathEscape(pathID)+"/resume",
				tt.body,
			)
			assertStatus(t, w, tt.status)
			phase18AssertNoPrompts(t, tt.id, tt.ordinal)
		})
	}
}

func TestPhase18MessagePointForkPromptLifecycle(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("USERPROFILE", cacheRoot)
	projectDir := t.TempDir()
	te := setup(t)
	te.seedSession(t, "phase18-life", projectDir, 2, func(s *db.Session) {
		s.Agent = "claude"
	})
	te.seedMessages(t, "phase18-life", 2)

	for range 2 {
		w := te.post(t,
			"/api/v1/sessions/phase18-life/resume",
			`{"command_only":true,"from_ordinal":1,"fork_session":true}`,
		)
		assertStatus(t, w, http.StatusOK)
	}
	matches, err := filepath.Glob(phase18PromptGlob(t, "phase18-life", 1))
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.NotEqual(t, matches[0], matches[1])
	for _, match := range matches {
		if runtime.GOOS != "windows" {
			info, err := os.Stat(match)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		}
		_ = os.Remove(match)
	}

	remote := setupPGMode(t)
	remote.seedSession(t, "phase18-life-remote", projectDir, 2, func(s *db.Session) {
		s.Agent = "claude"
	})
	remote.seedMessages(t, "phase18-life-remote", 2)
	w := remote.post(t,
		"/api/v1/sessions/phase18-life-remote/resume",
		`{"from_ordinal":1,"fork_session":true}`,
	)
	assertStatus(t, w, http.StatusNotImplemented)
	phase18AssertNoPrompts(t, "phase18-life-remote", 1)
}

func TestPhase18MessagePointForkTerminalFailureKeepsFallbackPrompt(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("USERPROFILE", cacheRoot)
	projectDir := t.TempDir()
	binDir := t.TempDir()
	te := setup(t, func(c *config.Config) {
		c.Terminal.Mode = "custom"
		c.Terminal.CustomBin = filepath.Join(binDir, "missing-terminal")
	})
	te.seedSession(t, "phase18-terminal-failure", projectDir, 2, func(s *db.Session) {
		s.Agent = "claude"
	})
	te.seedMessages(t, "phase18-terminal-failure", 2)

	w := te.post(t,
		"/api/v1/sessions/phase18-terminal-failure/resume",
		`{"from_ordinal":1,"fork_session":true}`,
	)
	assertStatus(t, w, http.StatusOK)
	var resp struct {
		Launched bool   `json:"launched"`
		Command  string `json:"command"`
		Error    string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Launched)
	assert.Equal(t, "no_terminal_found", resp.Error)
	require.NotEmpty(t, resp.Command)
	promptPath := phase18SinglePrompt(t, "phase18-terminal-failure", 1)
	phase18AssertCommandConsumesPrompt(t, resp.Command, promptPath)
	assert.FileExists(t, promptPath)
}

func TestPhase18MessagePointForkNoEngineCommandOnlyReturnsPrompt(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("USERPROFILE", cacheRoot)
	projectDir := t.TempDir()
	remote := setupPGMode(t)
	remote.seedSession(t, "phase18-commandonly-remote", projectDir, 2, func(s *db.Session) {
		s.Agent = "claude"
	})
	remote.seedMessages(t, "phase18-commandonly-remote", 2)

	w := remote.post(t,
		"/api/v1/sessions/phase18-commandonly-remote/resume",
		`{"command_only":true,"from_ordinal":1,"fork_session":true}`,
	)
	assertStatus(t, w, http.StatusOK)
	var resp struct {
		Launched bool   `json:"launched"`
		Command  string `json:"command"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Launched)
	require.NotEmpty(t, resp.Command)
	promptPath := phase18SinglePrompt(t, "phase18-commandonly-remote", 1)
	phase18AssertCommandConsumesPrompt(t, resp.Command, promptPath)
	assert.FileExists(t, promptPath)
}

func TestPhase18OpenAPIGeneratedResumeRequestHasFromOrdinal(t *testing.T) {
	te := setup(t)
	w := te.get(t, "/api/openapi.json")
	assertStatus(t, w, http.StatusOK)
	assert.Contains(t, w.Body.String(), "from_ordinal")
}

func TestGetSessionDirectory(t *testing.T) {
	te := setup(t)

	projectDir := t.TempDir()
	te.seedSession(t, "dir-1", projectDir, 3)

	t.Run("returns resolved directory", func(t *testing.T) {
		w := te.get(t, "/api/v1/sessions/dir-1/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assertSamePath(t, "path", resp.Path, projectDir)
	})

	t.Run("empty path for relative project", func(t *testing.T) {
		te.seedSession(t, "dir-2", "my-repo", 3)
		w := te.get(t, "/api/v1/sessions/dir-2/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Empty(t, resp.Path)
	})

	t.Run("not found", func(t *testing.T) {
		w := te.get(t, "/api/v1/sessions/nonexistent/directory")
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("prefers session file cwd", func(t *testing.T) {
		cwdDir := filepath.Join(t.TempDir(), "nested")
		require.NoError(t, os.Mkdir(cwdDir, 0o755))
		sessionFile := filepath.Join(t.TempDir(), "session.jsonl")
		cwdJSON, _ := json.Marshal(cwdDir)
		content := `{"cwd":` + string(cwdJSON) + "}\n"
		require.NoError(t, os.WriteFile(sessionFile, []byte(content), 0o644))
		te.seedSession(t, "dir-3", projectDir, 3, func(s *db.Session) {
			s.FilePath = &sessionFile
		})
		w := te.get(t, "/api/v1/sessions/dir-3/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assertSamePath(t, "path", resp.Path, cwdDir)
	})

	t.Run("cursor directory returns workspace root", func(t *testing.T) {
		projectDir := t.TempDir()
		runDir := filepath.Join(projectDir, "frontend")
		require.NoError(t, os.MkdirAll(runDir, 0o755))
		runDirJSON, _ := json.Marshal(runDir)
		sessionFile := filepath.Join(t.TempDir(), "cursor.jsonl")
		content := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Shell","input":{"command":"pwd","working_directory":` +
			string(runDirJSON) + `}}]}}` + "\n"
		require.NoError(t, os.WriteFile(sessionFile, []byte(content), 0o644))
		te.seedSession(t, "dir-cursor", projectDir, 3, func(s *db.Session) {
			s.Agent = "cursor"
			s.FilePath = &sessionFile
		})

		w := te.get(t, "/api/v1/sessions/dir-cursor/directory")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Path string `json:"path"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assertSamePath(t, "path", resp.Path, projectDir)
	})
}

func TestListOpeners(t *testing.T) {
	te := setup(t)

	w := te.get(t, "/api/v1/openers")
	assertStatus(t, w, http.StatusOK)

	var resp struct {
		Openers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Kind string `json:"kind"`
			Bin  string `json:"bin"`
		} `json:"openers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// The response should always be an array (possibly empty),
	// never null.
	assert.NotNil(t, resp.Openers, "openers should be [] not null")
}

func TestGetTerminalConfig(t *testing.T) {
	te := setup(t)

	t.Run("default config", func(t *testing.T) {
		w := te.get(t, "/api/v1/config/terminal")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Mode string `json:"mode"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "auto", resp.Mode)
	})

	t.Run("set and get", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/config/terminal",
			`{"mode":"clipboard"}`,
		)
		assertStatus(t, w, http.StatusOK)

		w = te.get(t, "/api/v1/config/terminal")
		assertStatus(t, w, http.StatusOK)
		var resp struct {
			Mode string `json:"mode"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "clipboard", resp.Mode)
	})

	t.Run("invalid mode", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/config/terminal",
			`{"mode":"invalid"}`,
		)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("custom requires bin", func(t *testing.T) {
		w := te.post(t,
			"/api/v1/config/terminal",
			`{"mode":"custom","custom_bin":""}`,
		)
		assertStatus(t, w, http.StatusBadRequest)
	})
}
