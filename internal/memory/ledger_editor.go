package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LedgerEditor edits explicit assist-mem JSONL entries in-place. The JSONL
// ledger remains the SSOT; deletes are soft deletes by setting status=archived.
type LedgerEditor struct {
	path string
}

func NewLedgerEditor(path string) *LedgerEditor {
	return &LedgerEditor{path: path}
}

func assistMemIDFromRelPath(relPath string) (string, error) {
	if !strings.HasPrefix(relPath, "assist-mem/") || !strings.HasSuffix(relPath, ".jsonl") {
		return "", ErrPathTraversal
	}
	id := strings.TrimSuffix(strings.TrimPrefix(relPath, "assist-mem/"), ".jsonl")
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return "", ErrPathTraversal
	}
	return id, nil
}

func (e *LedgerEditor) Read(_ context.Context, relPath string) (string, string, error) {
	id, err := assistMemIDFromRelPath(relPath)
	if err != nil {
		return "", "", err
	}
	lines, idx, err := e.readLines(id)
	if err != nil {
		return "", "", err
	}
	content := strings.TrimSpace(lines[idx])
	return content, sha256Hex([]byte(content)), nil
}

func (e *LedgerEditor) Write(_ context.Context, req WriteRequest) (string, error) {
	id, err := assistMemIDFromRelPath(req.RelPath)
	if err != nil {
		return "", err
	}
	var entry ledgerEntry
	if err := json.Unmarshal([]byte(req.Content), &entry); err != nil {
		return "", fmt.Errorf("invalid assist-mem JSON: %w", err)
	}
	if strings.TrimSpace(entry.ID) != id {
		return "", fmt.Errorf("assist-mem entry id does not match rel_path")
	}
	lines, idx, err := e.readLines(id)
	if err != nil {
		return "", err
	}
	current := strings.TrimSpace(lines[idx])
	if req.BaseSHA != "" && sha256Hex([]byte(current)) != req.BaseSHA {
		return "", ErrConflict
	}
	next := strings.TrimSpace(req.Content)
	lines[idx] = next
	if err := e.writeLines(lines); err != nil {
		return "", err
	}
	return sha256Hex([]byte(next)), nil
}

func (e *LedgerEditor) Archive(ctx context.Context, relPath, baseSHA string) (string, error) {
	content, sha, err := e.Read(ctx, relPath)
	if err != nil {
		return "", err
	}
	if baseSHA != "" && sha != baseSHA {
		return "", ErrConflict
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(content), &entry); err != nil {
		return "", fmt.Errorf("invalid assist-mem JSON: %w", err)
	}
	entry["status"] = "archived"
	next, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return e.Write(ctx, WriteRequest{RelPath: relPath, Content: string(next), BaseSHA: sha})
}

func (e *LedgerEditor) Delete(ctx context.Context, relPath, baseSHA string) error {
	_, err := e.Archive(ctx, relPath, baseSHA)
	return err
}

func (e *LedgerEditor) readLines(id string) ([]string, int, error) {
	data, err := os.ReadFile(e.path)
	if err != nil {
		return nil, -1, err
	}
	text := strings.TrimRight(string(data), "\n")
	lines := []string{}
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry ledgerEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if strings.TrimSpace(entry.ID) == id {
			return lines, i, nil
		}
	}
	return nil, -1, os.ErrNotExist
}

func (e *LedgerEditor) writeLines(lines []string) error {
	dir := filepath.Dir(e.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".assist-mem-ledger-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, e.path); err != nil {
		return err
	}
	return nil
}
