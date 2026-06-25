package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestResolveBackupInterval_Default(t *testing.T) {
	cfg := Config{}
	if got := cfg.ResolveBackupInterval(); got != time.Hour {
		t.Fatalf("default interval = %v, want 1h", got)
	}
	cfg.BackupInterval = 30 * time.Minute
	if got := cfg.ResolveBackupInterval(); got != 30*time.Minute {
		t.Fatalf("configured interval = %v, want 30m", got)
	}
}

func TestResolveBackupWorkspaceDir_DefaultsUnderDataDir(t *testing.T) {
	cfg := Config{DataDir: "/home/u/.agentsview"}
	want := filepath.Join("/home/u/.agentsview", "memory-backup")
	if got := cfg.ResolveBackupWorkspaceDir(); got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}

func TestResolveBackupWorkspaceDir_ExplicitOverride(t *testing.T) {
	cfg := Config{DataDir: "/home/u/.agentsview", BackupWorkspaceDir: "/custom/ws"}
	if got := cfg.ResolveBackupWorkspaceDir(); got != "/custom/ws" {
		t.Fatalf("workspace = %q, want /custom/ws", got)
	}
}

func TestBackupEnabled_DefaultsOff(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BackupEnabled {
		t.Fatal("BackupEnabled should default to OFF (pushing memory is an opt-in)")
	}
}

func TestBackupEnv_ParsesEnabledAndInterval(t *testing.T) {
	t.Setenv("AGENTSVIEW_BACKUP_ENABLED", "true")
	t.Setenv("AGENTSVIEW_BACKUP_INTERVAL", "15m")
	t.Setenv("AGENTSVIEW_BACKUP_WORKSPACE", "/tmp/ws")
	cfg := Config{}
	cfg.loadEnv()
	if !cfg.BackupEnabled {
		t.Fatal("expected BackupEnabled true from env")
	}
	if !cfg.envBackupEnabledSet {
		t.Fatal("expected envBackupEnabledSet to be recorded")
	}
	if cfg.BackupInterval != 15*time.Minute {
		t.Fatalf("interval = %v, want 15m", cfg.BackupInterval)
	}
	if cfg.BackupWorkspaceDir != "/tmp/ws" {
		t.Fatalf("workspace = %q, want /tmp/ws", cfg.BackupWorkspaceDir)
	}
}
