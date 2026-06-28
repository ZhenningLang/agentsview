package consolidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	semantic "go.kenn.io/agentsview/internal/search"
)

func TestBuildUserPromptCarriesSimilarMemoryContext(t *testing.T) {
	prompt := BuildUserPrompt([]Candidate{{ID: "cand-1", ProblemType: "knowledge", Summary: "Use BACKGROUND_JOBS_ENABLED=false"}}, map[string][]ExistingNote{
		"cand-1": {{NoteID: "background-jobs.md", Title: "Background jobs", ProblemType: "decision", Status: "active", Excerpt: "Existing jobs note", Score: 0.91}},
	})

	assert.Contains(t, prompt, `"id":"cand-1"`)
	assert.Contains(t, prompt, `"similar_memories"`)
	assert.Contains(t, prompt, `"note_id":"background-jobs.md"`)
	assert.Contains(t, prompt, `"excerpt":"Existing jobs note"`)
}

func TestExistingNoteFromRecallHitUsesBasenameNoteID(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory", "user")
	require.NoError(t, os.MkdirAll(memoryDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "target.md"), []byte("# Target\n"), 0o600))

	note := ExistingNoteFromRecallHit(semantic.MemoryRecallHit{RelPath: "memory/user/target.md", Title: "Target"})

	assert.Equal(t, "target.md", note.NoteID)
	assert.FileExists(t, filepath.Join(memoryDir, note.NoteID))
	assert.False(t, strings.Contains(note.NoteID, string(filepath.Separator)))
	assert.NoFileExists(t, filepath.Join(memoryDir, "memory/user/target.md"))
}

func TestSystemPromptDocumentsDestructiveActionsAndNoteIDContract(t *testing.T) {
	assert.Contains(t, systemPrompt, "DELETE")
	assert.Contains(t, systemPrompt, "INVALIDATE")
	assert.Contains(t, systemPrompt, "similar_memories[].note_id")
}
