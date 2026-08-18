package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21ReadOnlyArchiveDoesNotCreateOrInitialize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "sessions.db")

	d, err := OpenReadOnly(path)
	if d != nil {
		require.NoError(t, d.Close())
	}
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Dir(path))
}

func TestPhase21ReadOnlyArchiveDoesNotWriteUserVersionOrSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	d := phase21WritableDB(t, path)
	insertSession(t, d, "readonly-session", "proj")
	require.NoError(t, d.Close())

	staleVersion := dataVersion - 1
	phase21ExecRaw(t, path, fmt.Sprintf("PRAGMA user_version = %d", staleVersion))
	before := phase21DBFileSnapshot(t, path)

	ro, err := OpenReadOnly(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ro.Close()) })

	assert.True(t, ro.ReadOnly())
	assert.True(t, ro.NeedsResync())
	page, err := ro.ListSessions(context.Background(), SessionFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Sessions, 1)
	assert.Equal(t, "readonly-session", page.Sessions[0].ID)

	err = ro.UpsertSession(Session{ID: "blocked", Project: "proj", Machine: "local", Agent: "claude"})
	require.ErrorIs(t, err, ErrReadOnly)
	err = ro.Update(func(tx *sql.Tx) error { return nil })
	require.ErrorIs(t, err, ErrReadOnly)
	_, err = ro.getWriter().Exec("UPDATE sessions SET project = ? WHERE id = ?", "x", "readonly-session")
	require.ErrorIs(t, err, ErrReadOnly)
	_, err = ro.getWriter().Begin()
	require.ErrorIs(t, err, ErrReadOnly)
	conn, err := ro.getWriter().Conn(context.Background())
	if conn != nil {
		require.NoError(t, conn.Close())
	}
	require.ErrorIs(t, err, ErrReadOnly)

	require.NoError(t, ro.CloseConnections())
	require.NoError(t, ro.Reopen())
	assert.True(t, ro.ReadOnly())
	assert.Equal(t, staleVersion, phase21UserVersion(t, path))
	assert.Equal(t, before, phase21DBFileSnapshot(t, path))
}

func TestPhase21ReadOnlyArchiveRejectsIncompatibleSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	phase21ExecRaw(t, path, `CREATE TABLE sessions (id TEXT PRIMARY KEY);`)

	ro, err := OpenReadOnly(path)
	if ro != nil {
		require.NoError(t, ro.Close())
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
	assert.Equal(t, 0, phase21UserVersion(t, path))
}

func phase21WritableDB(t *testing.T, path string) *DB {
	t.Helper()
	d, err := Open(path)
	require.NoError(t, err)
	return d
}

func phase21ExecRaw(t *testing.T, path string, stmt string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	_, err = conn.Exec(stmt)
	require.NoError(t, err)
}

func phase21UserVersion(t *testing.T, path string) int {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	var version int
	require.NoError(t, conn.QueryRow("PRAGMA user_version").Scan(&version))
	return version
}

func phase21DBFileSnapshot(t *testing.T, path string) map[string]struct {
	Size    int64
	ModUnix int64
} {
	t.Helper()
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	snapshot := map[string]struct {
		Size    int64
		ModUnix int64
	}{}
	for _, entry := range entries {
		name := entry.Name()
		if name != base && name != base+"-wal" && name != base+"-shm" {
			continue
		}
		info, err := entry.Info()
		require.NoError(t, err)
		snapshot[name] = struct {
			Size    int64
			ModUnix int64
		}{Size: info.Size(), ModUnix: info.ModTime().UnixNano()}
	}
	return snapshot
}

func TestPhase21ReadOnlyArchiveErrorIsStable(t *testing.T) {
	require.True(t, errors.Is(ErrReadOnly, ErrReadOnly))
	require.False(t, errors.Is(ErrReadOnly, os.ErrNotExist))
}
