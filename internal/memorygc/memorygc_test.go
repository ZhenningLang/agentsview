package memorygc

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedCall struct {
	dir  string
	name string
	args []string
}

type fakeRunner struct {
	calls []recordedCall
	err   error
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, recordedCall{dir: dir, name: name, args: args})
	return "", f.err
}

func TestRunOnceInvokesBothGCLegs(t *testing.T) {
	f := &fakeRunner{}
	root := filepath.Join(string(filepath.Separator), "df")
	gc := GC{Root: root, ArchivedNoteTTLDays: 0, Runner: f}

	require.NoError(t, gc.RunOnce(context.Background()))
	require.Len(t, f.calls, 2)

	// Leg 1: candidate + consumed GC via memory_capture --gc.
	c1 := f.calls[0]
	assert.Equal(t, root, c1.dir)
	assert.Contains(t, strings.Join(c1.args, " "), filepath.Join("scripts", "hooks", "memory_capture.py"))
	assert.Contains(t, c1.args, "--gc")

	// Leg 2: archived-note GC with the default TTL substituted for 0.
	c2 := f.calls[1]
	assert.Contains(t, strings.Join(c2.args, " "), "assist_consolidate.py")
	assert.Contains(t, c2.args, "--gc-archived-notes")
	joined := strings.Join(c2.args, " ")
	assert.Contains(t, joined, "--archived-note-ttl-days 90")
	assert.Contains(t, joined, filepath.Join("memory", ".staging", "raw_memories"))
}

func TestRunOnceRunsBothLegsEvenIfFirstFails(t *testing.T) {
	f := &fakeRunner{err: errors.New("boom")}
	root := filepath.Join(string(filepath.Separator), "df")
	gc := GC{Root: root, ArchivedNoteTTLDays: 30, Runner: f}

	err := gc.RunOnce(context.Background())
	assert.Error(t, err, "the first leg's error is surfaced")
	assert.Len(t, f.calls, 2, "the second leg still runs after the first fails")
	assert.Contains(t, strings.Join(f.calls[1].args, " "), "--archived-note-ttl-days 30")
}
