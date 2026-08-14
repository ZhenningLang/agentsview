//go:build !windows

package parser

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillNameFromPathRejectsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	fifoPath := filepath.Join(root, "skills", "fifo", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(fifoPath), 0o755))
	require.NoError(t, syscall.Mkfifo(fifoPath, 0o644))

	assert.Equal(t, "fifo", skillNameFromPath(fifoPath, ""),
		"non-regular files must not be opened for frontmatter")
}
