package server_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/server"
)

// These tests exercise the server-side source-routing wiring introduced in
// phase 02 (CC-native edit write-back). The memory writer unit tests prove the
// no-git writer works in isolation; these prove the route layer selects the
// correct root by a note's data source — the actual user-facing behavior of
// this phase. A cc-native PUT must land under the CC root (no-git), a
// cross-agent PUT under the SSOT root (git-backed), raw GET must read from the
// source-correct root, and cc-native history/at-commit/revert must report
// "not applicable".

// encodeMemPath mirrors the route's URL-safe base64 encoding of a rel_path.
func encodeMemPath(relPath string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(relPath))
}

func memSHA(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// memoryFixture builds two isolated memory roots and seeds the DB with one
// note per source so writerForRelPath / isCCNative can resolve them. The
// cross-agent root is a local-only git repo (the git-backed writer path); the
// CC-native root holds a note at <project>/memory/<file>.md (the no-git,
// multi-root path). Returns both roots and both rel_paths.
type memoryFixture struct {
	te           *testEnv
	ssotDir      string
	ccDir        string
	ledgerPath   string
	crossRelPath string
	ccRelPath    string
	crossContent string
	ccContent    string
}

func setupMemoryFixture(t *testing.T, opts ...setupOption) *memoryFixture {
	return setupMemoryFixtureWithServerOpts(t, nil, opts...)
}

func setupMemoryFixtureWithServerOpts(t *testing.T, srvOpts []server.Option, opts ...setupOption) *memoryFixture {
	t.Helper()

	ssotDir := t.TempDir()
	ccDir := t.TempDir()
	ledgerPath := filepath.Join(t.TempDir(), "entries.jsonl")

	// Cross-agent note: lives directly under the SSOT root and the root is a
	// local-only git repo (so the git-backed writer can commit/read history).
	crossRelPath := "cross.md"
	crossContent := "---\ntitle: Cross\ndate: 2026-06-20\nstatus: active\n---\n\nCross body.\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(ssotDir, crossRelPath), []byte(crossContent), 0o644))
	gitInitCommit(t, ssotDir)

	// CC-native note: lives at <project>/memory/<file>.md under the CC root,
	// which is NOT our git repo.
	ccRelPath := filepath.ToSlash(filepath.Join("proj-a", "memory", "note.md"))
	ccContent := "---\ntitle: CC Note\ndate: 2026-06-21\n---\n\nCC body.\n"
	ccFull := filepath.Join(ccDir, filepath.FromSlash(ccRelPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(ccFull), 0o755))
	require.NoError(t, os.WriteFile(ccFull, []byte(ccContent), 0o644))
	ledgerLine := `{"id":"abd80440ea5d8479","created_at":"2026-07-01T13:36:35Z","project":"ordo_ai","scope":"project","source":"explicit","type":"project-fact","status":"active","text":"Assist Mem body.","evidence":"initial evidence","triggers":["lzn"]}` + "\n"
	require.NoError(t, os.WriteFile(ledgerPath, []byte(ledgerLine), 0o644))

	setupOpts := append([]setupOption{withMemoryDir(ssotDir), withCCMemoryDir(ccDir), withAssistMemLedger(ledgerPath)}, opts...)
	te := setupWithServerOpts(t, srvOpts, setupOpts...)

	// Seed the DB rows the route reads to learn each note's source.
	ctx := context.Background()
	require.NoError(t, te.db.ReplaceMemoriesBySource(
		ctx, db.SourceCrossAgent, []db.Memory{{
			RelPath: crossRelPath,
			Source:  db.SourceCrossAgent,
			Title:   "Cross",
			Date:    "2026-06-20",
			Status:  "active",
		}}))
	require.NoError(t, te.db.ReplaceMemoriesBySource(
		ctx, db.SourceCCNative, []db.Memory{{
			RelPath: ccRelPath,
			Source:  db.SourceCCNative,
			Title:   "CC Note",
			Date:    "2026-06-21",
		}}))
	require.NoError(t, te.db.ReplaceMemoriesBySource(
		ctx, db.SourceAssistMem, []db.Memory{{
			RelPath: "assist-mem/abd80440ea5d8479.jsonl",
			Source:  db.SourceAssistMem,
			Title:   "Assist Mem",
			Date:    "2026-07-01",
			Status:  "active",
		}}))
	require.NoError(t, te.db.ReplaceMemoriesBySource(
		ctx, db.SourceCanonical, []db.Memory{{
			RelPath:              "canonical/entrypoint.json",
			Source:               db.SourceCanonical,
			Title:                "Canonical Entrypoint",
			Date:                 "2026-07-02",
			Status:               "active",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/abd80440ea5d8479.jsonl"}]`,
			CanonicalProvenance:  `{"topic":"entrypoint"}`,
		}}))

	return &memoryFixture{
		te:           te,
		ssotDir:      ssotDir,
		ccDir:        ccDir,
		ledgerPath:   ledgerPath,
		crossRelPath: crossRelPath,
		ccRelPath:    ccRelPath,
		crossContent: crossContent,
		ccContent:    ccContent,
	}
}

// gitInitCommit makes dir a local-only git repo with all its files committed.
func gitInitCommit(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// putMemory issues an authenticated PUT to the memory write-back route.
func (te *testEnv) putMemory(
	t *testing.T, relPath, content, baseSHA string,
) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"content":` + jsonString(content) +
		`,"base_sha":` + jsonString(baseSHA) + `}`
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/memories/"+encodeMemPath(relPath),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

func (te *testEnv) deleteMemory(t *testing.T, relPath, baseSHA string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/memories/"+encodeMemPath(relPath)+"?base_sha="+baseSHA,
		nil)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

func (te *testEnv) postMemoryFeedback(
	t *testing.T, relPath, vote, comment, status string,
) *httptest.ResponseRecorder {
	t.Helper()
	return te.postMemoryFeedbackWithBaseSHA(t, relPath, vote, comment, status, "")
}

func (te *testEnv) postMemoryFeedbackWithBaseSHA(
	t *testing.T, relPath, vote, comment, status, baseSHA string,
) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"vote":` + jsonString(vote) +
		`,"comment":` + jsonString(comment) +
		`,"status":` + jsonString(status) +
		`,"base_sha":` + jsonString(baseSHA) + `}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/memories/"+encodeMemPath(relPath)+"/feedback",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

// jsonString returns a minimal JSON-quoted string. Test inputs avoid control
// chars, so quoting backslash and double-quote is sufficient.
func jsonString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}

// TestMemoryPut_CCNativeLandsInCCRoot is the core phase behavior: a cc-native
// PUT must write under the CC root (not the SSOT root), and must NOT create a
// git commit there (no-git path).
func TestMemoryPut_CCNativeLandsInCCRoot(t *testing.T) {
	fx := setupMemoryFixture(t)

	newContent := "---\ntitle: CC Note\ndate: 2026-06-21\n---\n\nedited cc body.\n"
	w := fx.te.putMemory(t, fx.ccRelPath, newContent, memSHA(fx.ccContent))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	got := decode[struct {
		SHA string `json:"sha"`
	}](t, w)
	assert.Equal(t, memSHA(newContent), got.SHA)

	// The file under the CC root changed.
	ccFull := filepath.Join(fx.ccDir, filepath.FromSlash(fx.ccRelPath))
	onDisk, err := os.ReadFile(ccFull)
	require.NoError(t, err)
	assert.Equal(t, newContent, string(onDisk),
		"cc-native PUT must write to the CC root")

	// Nothing must have leaked into the SSOT root: no file with the cc-native
	// basename appears there, and the SSOT note is untouched.
	ssotCross, err := os.ReadFile(filepath.Join(fx.ssotDir, fx.crossRelPath))
	require.NoError(t, err)
	assert.Equal(t, fx.crossContent, string(ssotCross),
		"cross-agent note must be untouched by a cc-native PUT")

	// The CC root is not a git repo (no-git writer): no .git dir was created.
	_, statErr := os.Stat(filepath.Join(fx.ccDir, ".git"))
	assert.True(t, os.IsNotExist(statErr),
		"cc-native write must not create a git repo")
}

// TestMemoryPut_CrossAgentLandsInSSOTRoot proves the other arm of the switch:
// a cross-agent PUT writes under the SSOT root and is committed to its
// local-only git repo.
func TestMemoryPut_CrossAgentLandsInSSOTRoot(t *testing.T) {
	fx := setupMemoryFixture(t)

	newContent := "---\ntitle: Cross\ndate: 2026-06-20\n---\n\nedited cross body.\n"
	w := fx.te.putMemory(t, fx.crossRelPath, newContent, memSHA(fx.crossContent))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	onDisk, err := os.ReadFile(filepath.Join(fx.ssotDir, fx.crossRelPath))
	require.NoError(t, err)
	assert.Equal(t, newContent, string(onDisk),
		"cross-agent PUT must write to the SSOT root")

	// The SSOT root is a git repo and the edit was committed (HEAD subject
	// mentions the edited rel_path).
	out, gerr := exec.Command(
		"git", "-C", fx.ssotDir, "log", "-1", "--pretty=%s").CombinedOutput()
	require.NoError(t, gerr, "git log: %s", out)
	assert.Contains(t, string(out), fx.crossRelPath,
		"cross-agent write must commit the local git repo")
}

func TestMemoryPut_AssistMemWritesLedgerEntry(t *testing.T) {
	fx := setupMemoryFixture(t)
	relPath := "assist-mem/abd80440ea5d8479.jsonl"

	wRaw := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(relPath)+"/raw")
	require.Equal(t, http.StatusOK, wRaw.Code, "body: %s", wRaw.Body.String())
	raw := decode[struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}](t, wRaw)

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw.Content), &entry))
	entry["text"] = "Edited assist-mem body."
	entry["evidence"] = "edited evidence"
	edited, err := json.Marshal(entry)
	require.NoError(t, err)

	w := fx.te.putMemory(t, relPath, string(edited), raw.SHA)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	ledger, err := os.ReadFile(fx.ledgerPath)
	require.NoError(t, err)
	assert.Contains(t, string(ledger), "Edited assist-mem body.")
	assert.Contains(t, string(ledger), "edited evidence")
}

func TestMemoryDelete_AssistMemArchivesLedgerEntry(t *testing.T) {
	fx := setupMemoryFixture(t)
	relPath := "assist-mem/abd80440ea5d8479.jsonl"

	wRaw := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(relPath)+"/raw")
	require.Equal(t, http.StatusOK, wRaw.Code, "body: %s", wRaw.Body.String())
	raw := decode[struct {
		SHA string `json:"sha"`
	}](t, wRaw)

	w := fx.te.deleteMemory(t, relPath, raw.SHA)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	ledger, err := os.ReadFile(fx.ledgerPath)
	require.NoError(t, err)
	var entry struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(ledger), &entry))
	assert.Equal(t, "archived", entry.Status)
}

func TestMemoryDelete_CrossAgentDeletesFile(t *testing.T) {
	fx := setupMemoryFixture(t)

	w := fx.te.deleteMemory(t, fx.crossRelPath, memSHA(fx.crossContent))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	_, err := os.Stat(filepath.Join(fx.ssotDir, fx.crossRelPath))
	assert.True(t, os.IsNotExist(err), "cross-agent DELETE must remove the SSOT file")
	out, gerr := exec.Command(
		"git", "-C", fx.ssotDir, "log", "-1", "--pretty=%s").CombinedOutput()
	require.NoError(t, gerr, "git log: %s", out)
	assert.Contains(t, string(out), "memory: delete "+fx.crossRelPath)
}

func TestMemoryDelete_CanonicalRejected(t *testing.T) {
	fx := setupMemoryFixture(t)
	w := fx.te.deleteMemory(t, "canonical/entrypoint.json", "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

func TestMemoryFeedbackWritesQuotedFrontmatterAndResyncs(t *testing.T) {
	fx := setupMemoryFixture(t)

	w := fx.te.postMemoryFeedback(t, fx.crossRelPath, "down", "原因: 过度合并", "pending")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := decode[struct {
		SHA string `json:"sha"`
	}](t, w)
	require.NotEmpty(t, got.SHA)

	onDisk, err := os.ReadFile(filepath.Join(fx.ssotDir, fx.crossRelPath))
	require.NoError(t, err)
	assert.Contains(t, string(onDisk), "feedback_vote: down")
	assert.Contains(t, string(onDisk), `feedback_comment: "原因: 过度合并"`)
	assert.Contains(t, string(onDisk), "feedback_status: pending")

	wGet := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(fx.crossRelPath))
	require.Equal(t, http.StatusOK, wGet.Code, "body: %s", wGet.Body.String())
	mem := decode[db.Memory](t, wGet)
	assert.Equal(t, "down", mem.FeedbackVote)
	assert.Equal(t, "原因: 过度合并", mem.FeedbackComment)
	assert.Equal(t, "pending", mem.FeedbackStatus)
}

func TestMemoryFeedbackResyncPreservesEmbeddingWhenEmbedConfigMissing(t *testing.T) {
	fx := setupMemoryFixture(t)
	ctx := context.Background()
	existingVector := []float32{0.2, 0.8}
	require.NoError(t, fx.te.db.ReplaceMemoriesBySource(ctx, db.SourceCrossAgent, []db.Memory{{
		RelPath:         fx.crossRelPath,
		Source:          db.SourceCrossAgent,
		Title:           "Cross",
		Date:            "2026-06-20",
		Status:          "active",
		Body:            "Cross body.\n",
		SourceMtime:     fileMTime(t, filepath.Join(fx.ssotDir, fx.crossRelPath)),
		LLMEmbedding:    existingVector,
		LLMEmbeddingDim: len(existingVector),
	}}))

	w := fx.te.postMemoryFeedback(t, fx.crossRelPath, "down", "", "pending")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	embeddings, err := fx.te.db.MemoryEmbeddings(ctx, db.MemoryFilter{Source: db.SourceCrossAgent})
	require.NoError(t, err)
	require.Len(t, embeddings, 1)
	assert.Equal(t, existingVector, embeddings[0].LLMEmbedding)
	assert.Equal(t, len(existingVector), embeddings[0].LLMEmbeddingDim)
}

func TestMemoryPutResyncFallsBackToLexicalWhenEmbeddingProviderFails(t *testing.T) {
	called := 0
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		called++
		assert.Equal(t, "/v1/embeddings", req.URL.Path)
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":"down"}`)),
		}, nil
	})
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	fx := setupMemoryFixtureWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.Embed.BaseURL = "https://embed.example.test/v1"
		c.Embed.Model = "text-embedding"
	}))
	newContent := "---\ntitle: Cross\ndate: 2026-06-20\nstatus: active\n---\n\nEdited cross body.\n"

	w := fx.te.putMemory(t, fx.crossRelPath, newContent, memSHA(fx.crossContent))
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Positive(t, called, "configured embedder should be attempted before lexical fallback")
	assert.Contains(t, logs.String(), "retrying lexical sync")

	wGet := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(fx.crossRelPath))
	require.Equal(t, http.StatusOK, wGet.Code, "body: %s", wGet.Body.String())
	mem := decode[db.Memory](t, wGet)
	assert.Contains(t, mem.Body, "Edited cross body.")
}

func fileMTime(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.ModTime().Unix()
}

func TestMemoryFeedbackRejectsInvalidVoteAndStatus(t *testing.T) {
	fx := setupMemoryFixture(t)

	wVote := fx.te.postMemoryFeedback(t, fx.crossRelPath, "meh", "", "pending")
	assert.Equal(t, http.StatusBadRequest, wVote.Code, "body: %s", wVote.Body.String())

	wStatus := fx.te.postMemoryFeedback(t, fx.crossRelPath, "up", "", "open")
	assert.Equal(t, http.StatusBadRequest, wStatus.Code, "body: %s", wStatus.Body.String())
}

func TestAssistMemIsEditableButHasNoGitHistory(t *testing.T) {
	fx := setupMemoryFixture(t)
	relPath := "assist-mem/abd80440ea5d8479.jsonl"

	wRaw := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(relPath)+"/raw")
	require.Equal(t, http.StatusOK, wRaw.Code, "body: %s", wRaw.Body.String())
	raw := decode[struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}](t, wRaw)

	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw.Content), &entry))
	entry["text"] = "Editable assist-mem entry."
	edited, err := json.Marshal(entry)
	require.NoError(t, err)
	wPut := fx.te.putMemory(t, relPath, string(edited), raw.SHA)
	assert.Equal(t, http.StatusOK, wPut.Code, "body: %s", wPut.Body.String())

	wHistory := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(relPath)+"/history")
	require.Equal(t, http.StatusOK, wHistory.Code, "body: %s", wHistory.Body.String())
	got := decode[struct {
		History []any `json:"history"`
	}](t, wHistory)
	assert.Empty(t, got.History)
}

func TestCanonicalMemoryIsReadOnlyAndHasNoHistory(t *testing.T) {
	fx := setupMemoryFixture(t)
	relPath := "canonical/entrypoint.json"

	wRaw := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(relPath)+"/raw")
	assert.Equal(t, http.StatusBadRequest, wRaw.Code)

	wPut := fx.te.putMemory(t, relPath, "edited", "")
	assert.Equal(t, http.StatusBadRequest, wPut.Code)

	wFeedback := fx.te.postMemoryFeedback(t, relPath, "up", "", "handled")
	assert.Equal(t, http.StatusBadRequest, wFeedback.Code)

	wHistory := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(relPath)+"/history")
	require.Equal(t, http.StatusOK, wHistory.Code, "body: %s", wHistory.Body.String())
	got := decode[struct {
		History []any `json:"history"`
	}](t, wHistory)
	assert.Empty(t, got.History)
}

func TestMemoryListFiltersAssistMemSource(t *testing.T) {
	fx := setupMemoryFixture(t)

	w := fx.te.get(t, "/api/v1/memories?source=assist-mem")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := decode[struct {
		Memories []db.Memory `json:"memories"`
	}](t, w)

	require.Len(t, got.Memories, 1)
	assert.Equal(t, db.SourceAssistMem, got.Memories[0].Source)
	assert.Equal(t, "assist-mem/abd80440ea5d8479.jsonl", got.Memories[0].RelPath)
}

func TestMemoryListFiltersCanonicalSource(t *testing.T) {
	fx := setupMemoryFixture(t)

	w := fx.te.get(t, "/api/v1/memories?source=canonical")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := decode[struct {
		Memories []db.Memory `json:"memories"`
	}](t, w)

	require.Len(t, got.Memories, 1)
	assert.Equal(t, db.SourceCanonical, got.Memories[0].Source)
	assert.Equal(t, "canonical/entrypoint.json", got.Memories[0].RelPath)
	assert.Equal(t, `[{"source":"assist-mem","rel_path":"assist-mem/abd80440ea5d8479.jsonl"}]`, got.Memories[0].CanonicalCoveredRefs)
	assert.Equal(t, `{"topic":"entrypoint"}`, got.Memories[0].CanonicalProvenance)
}

func TestMemoryFeedbackReportsConflictWhenDiskChangedAfterDBSnapshot(t *testing.T) {
	fx := setupMemoryFixture(t)
	rel := fx.crossRelPath
	path := filepath.Join(fx.ssotDir, rel)
	changed := strings.Replace(fx.crossContent, "Cross body.", "external edit.", 1)
	require.NoError(t, os.WriteFile(path, []byte(changed), 0o644))

	w := fx.te.postMemoryFeedbackWithBaseSHA(t, rel, "up", "", "pending", memSHA(fx.crossContent))
	assert.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
}

// TestMemoryRaw_RoutesBySource covers the raw-GET behavior change: the raw read
// must come from the source-correct root. Previously raw GET always used the
// cross-agent dir, so a cc-native raw read would have read the wrong tree.
func TestMemoryRaw_RoutesBySource(t *testing.T) {
	fx := setupMemoryFixture(t)

	wCC := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(fx.ccRelPath)+"/raw")
	require.Equal(t, http.StatusOK, wCC.Code, "body: %s", wCC.Body.String())
	rawCC := decode[struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}](t, wCC)
	assert.Equal(t, fx.ccContent, rawCC.Content,
		"cc-native raw GET must read from the CC root")
	assert.Equal(t, memSHA(fx.ccContent), rawCC.SHA)

	wCross := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(fx.crossRelPath)+"/raw")
	require.Equal(t, http.StatusOK, wCross.Code, "body: %s", wCross.Body.String())
	rawCross := decode[struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}](t, wCross)
	assert.Equal(t, fx.crossContent, rawCross.Content,
		"cross-agent raw GET must read from the SSOT root")
}

// TestMemoryHistory_CCNativeEmpty asserts cc-native history is reported as an
// empty list ("not applicable" — no git repo), while a cross-agent note still
// returns its real git history.
func TestMemoryHistory_CCNativeEmpty(t *testing.T) {
	fx := setupMemoryFixture(t)

	wCC := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(fx.ccRelPath)+"/history")
	require.Equal(t, http.StatusOK, wCC.Code, "body: %s", wCC.Body.String())
	histCC := decode[struct {
		History []struct {
			Commit string `json:"commit"`
		} `json:"history"`
	}](t, wCC)
	assert.Empty(t, histCC.History,
		"cc-native history must be an empty list")

	wCross := fx.te.get(t, "/api/v1/memories/"+encodeMemPath(fx.crossRelPath)+"/history")
	require.Equal(t, http.StatusOK, wCross.Code, "body: %s", wCross.Body.String())
	histCross := decode[struct {
		History []struct {
			Commit string `json:"commit"`
		} `json:"history"`
	}](t, wCross)
	assert.NotEmpty(t, histCross.History,
		"cross-agent history must return real git commits")
}

// TestMemoryHistoryActions_CCNative400 asserts at-commit and revert reject
// cc-native notes with a 400 (no git history to act on).
func TestMemoryHistoryActions_CCNative400(t *testing.T) {
	fx := setupMemoryFixture(t)

	wAt := fx.te.get(t,
		"/api/v1/memories/"+encodeMemPath(fx.ccRelPath)+"/history/deadbeef")
	assert.Equal(t, http.StatusBadRequest, wAt.Code,
		"cc-native at-commit must 400; body: %s", wAt.Body.String())

	body := `{"commit":"deadbeef","base_sha":""}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/memories/"+encodeMemPath(fx.ccRelPath)+"/revert",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	wRevert := httptest.NewRecorder()
	fx.te.handler.ServeHTTP(wRevert, req)
	assert.Equal(t, http.StatusBadRequest, wRevert.Code,
		"cc-native revert must 400; body: %s", wRevert.Body.String())
}
