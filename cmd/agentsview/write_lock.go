package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

const writeOwnerLockFile = "db.write.lock"

type writeOwnerLock struct {
	path string
	lock *flock.Flock
}

var writeOwnerLocks sync.Map

func writeOwnerLockPath(dataDir string) string {
	return filepath.Join(dataDir, writeOwnerLockFile)
}

func writeLockDataDir(cfg config.Config) string {
	if cfg.DataDir != "" {
		return cfg.DataDir
	}
	return filepath.Dir(cfg.DBPath)
}

func acquireWriteOwnerLock(ctx context.Context, dataDir string) (*writeOwnerLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir for write lock: %w", err)
	}
	path := writeOwnerLockPath(dataDir)
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquiring SQLite write-owner lock %s: %w", path, err)
	}
	if !locked {
		return nil, fmt.Errorf(
			"SQLite archive is already owned by another agentsview writer (%s); "+
				"run `agentsview daemon stop` if a daemon owns it, or retry after the current writer exits",
			path,
		)
	}
	return &writeOwnerLock{path: path, lock: lock}, nil
}

func (l *writeOwnerLock) Close() error {
	if l == nil || l.lock == nil {
		return nil
	}
	return l.lock.Unlock()
}

func openWriteDB(ctx context.Context, cfg config.Config) (*db.DB, error) {
	if err := validateWriteOwnership(cfg); err != nil {
		return nil, err
	}
	owner, err := acquireWriteOwnerLock(ctx, writeLockDataDir(cfg))
	if err != nil {
		return nil, err
	}
	applyClassifierConfig(cfg)
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		_ = owner.Close()
		return nil, err
	}
	if err := configureDBFromConfig(database, cfg); err != nil {
		_ = database.Close()
		_ = owner.Close()
		return nil, err
	}
	writeOwnerLocks.Store(database, owner)
	return database, nil
}

func validateWriteOwnership(cfg config.Config) error {
	dataDir := writeLockDataDir(cfg)
	if rt := FindDaemonRuntime(dataDir, cfg.AuthToken); rt != nil && !rt.ReadOnly {
		return fmt.Errorf(
			"local daemon at %s owns the SQLite archive; run `agentsview daemon stop` "+
				"or retry the direct writer later",
			urlFromDaemonRuntime(rt),
		)
	}
	if IsDaemonStarting(dataDir) && !currentProcessOwnsStartLock(dataDir) {
		return fmt.Errorf(
			"local daemon is still starting and may own the SQLite archive; "+
				"run `agentsview daemon stop` if startup is stuck, or retry later",
		)
	}
	return nil
}

func currentProcessOwnsStartLock(dataDir string) bool {
	path, err := runtimeStore(dataDir).LockPath()
	if err != nil {
		return false
	}
	_, ok := startLocks.Load(path)
	return ok
}

func openReadOnlyDB(cfg config.Config) (*db.DB, error) {
	applyClassifierConfig(cfg)
	database, err := db.OpenReadOnly(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := configureDBFromConfig(database, cfg); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func configureDBFromConfig(database *db.DB, cfg config.Config) error {
	applyCustomPricing(database, cfg)
	if cfg.CursorSecret == "" {
		return nil
	}
	secret, err := base64.StdEncoding.DecodeString(cfg.CursorSecret)
	if err != nil {
		return fmt.Errorf("invalid cursor secret: %w", err)
	}
	database.SetCursorSecret(secret)
	return nil
}

func closeWriteDB(database *db.DB) error {
	if database == nil {
		return nil
	}
	err := database.Close()
	if value, ok := writeOwnerLocks.LoadAndDelete(database); ok {
		if lockErr := value.(*writeOwnerLock).Close(); err == nil {
			err = lockErr
		}
	}
	return err
}
