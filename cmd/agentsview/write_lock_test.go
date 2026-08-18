package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestPhase21WriteOwnerLockAcquireReleaseAndStaleFile(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(writeOwnerLockPath(dataDir), []byte("stale"), 0o644))

	first, err := acquireWriteOwnerLock(context.Background(), dataDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dataDir, writeOwnerLockFile), first.path)

	second, err := acquireWriteOwnerLock(context.Background(), dataDir)
	if second != nil {
		require.NoError(t, second.Close())
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentsview daemon stop")
	assert.Contains(t, err.Error(), "retry")

	require.NoError(t, first.Close())
	again, err := acquireWriteOwnerLock(context.Background(), dataDir)
	require.NoError(t, err)
	require.NoError(t, again.Close())
}

func TestPhase21WriteOwnerLockContextCanceledFailsFast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lock, err := acquireWriteOwnerLock(ctx, t.TempDir())
	if lock != nil {
		require.NoError(t, lock.Close())
	}
	require.ErrorIs(t, err, context.Canceled)
}

func TestPhase21WriteOwnerLockConcurrentAcquireOnlyOneSucceeds(t *testing.T) {
	dataDir := t.TempDir()
	start := make(chan struct{})
	locks := make(chan *writeOwnerLock, 2)
	errs := make(chan error, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			lock, err := acquireWriteOwnerLock(context.Background(), dataDir)
			locks <- lock
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(locks)
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	for lock := range locks {
		if lock != nil {
			require.NoError(t, lock.Close())
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)
}

func TestPhase21WriteOwnerLockSurvivesOwnerCrash(t *testing.T) {
	if os.Getenv("AGENTSVIEW_PHASE21_HOLD_WRITE_LOCK") == "1" {
		lock, err := acquireWriteOwnerLock(context.Background(), os.Getenv("AGENTSVIEW_PHASE21_LOCK_DIR"))
		if err != nil {
			os.Exit(2)
		}
		defer lock.Close()
		_, _ = os.Stdout.Write([]byte("locked\n"))
		select {}
	}

	dataDir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPhase21WriteOwnerLockSurvivesOwnerCrash$")
	cmd.Env = append(os.Environ(),
		"AGENTSVIEW_PHASE21_HOLD_WRITE_LOCK=1",
		"AGENTSVIEW_PHASE21_LOCK_DIR="+dataDir,
	)
	require.NoError(t, cmd.Start())
	require.Eventually(t, func() bool {
		lock, err := acquireWriteOwnerLock(context.Background(), dataDir)
		if err == nil {
			require.NoError(t, lock.Close())
			return false
		}
		return true
	}, 5*time.Second, 25*time.Millisecond)

	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	require.Eventually(t, func() bool {
		lock, err := acquireWriteOwnerLock(context.Background(), dataDir)
		if err != nil {
			return false
		}
		require.NoError(t, lock.Close())
		return true
	}, 5*time.Second, 25*time.Millisecond)
}

func TestPhase21WriteOwnerOpenWriteDBRejectsHeldLockAndClosesInOrder(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{DataDir: dataDir, DBPath: filepath.Join(dataDir, "sessions.db")}

	first, err := openWriteDB(context.Background(), cfg)
	require.NoError(t, err)
	second, err := openWriteDB(context.Background(), cfg)
	if second != nil {
		require.NoError(t, closeWriteDB(second))
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentsview daemon stop")

	require.NoError(t, closeWriteDB(first))
	again, err := openWriteDB(context.Background(), cfg)
	require.NoError(t, err)
	require.NoError(t, closeWriteDB(again))
}
