package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21DaemonCheckDataVersionRejectsNewerArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	d, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, d.Close())
	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	_, err = conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", dataVersion+1))
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	err = CheckDataVersion(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than supported version")
}

func TestPhase21DaemonCheckDataVersionAllowsMissingArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "sessions.db")
	assert.NoError(t, CheckDataVersion(path))
}
