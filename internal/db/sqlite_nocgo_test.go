//go:build !cgo

package db

import (
	"strings"
	"testing"
)

func TestOpenNoCGOReturnsRequiresCGOError(t *testing.T) {
	_, err := Open(t.TempDir() + "/sessions.db")
	if err == nil {
		t.Fatal("Open without cgo succeeded, want requires-cgo error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "requires cgo") {
		t.Fatalf("Open without cgo error = %q, want requires-cgo message", err)
	}
}
