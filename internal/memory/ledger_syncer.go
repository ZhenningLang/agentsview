package memory

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/skills"
)

// LedgerSyncer mirrors the explicit assist-mem JSONL ledger into the memory
// dimension table. It is read-only against the ledger and replaces only the
// assist-mem source, so legacy cross-agent and cc-native rows are untouched.
type LedgerSyncer struct {
	path      string
	tokenizer skills.Tokenizer
	writer    Writer
	now       func() time.Time
}

type ledgerEntry struct {
	CreatedAt string   `json:"created_at"`
	Evidence  string   `json:"evidence"`
	ID        string   `json:"id"`
	Project   string   `json:"project"`
	Scope     string   `json:"scope"`
	Source    string   `json:"source"`
	Status    string   `json:"status"`
	Text      string   `json:"text"`
	Triggers  []string `json:"triggers"`
	Type      string   `json:"type"`
}

func NewLedgerSyncer(path string, w Writer, tk skills.Tokenizer) *LedgerSyncer {
	if tk == nil {
		tk = skills.NewHeuristicTokenizer()
	}
	return &LedgerSyncer{path: path, writer: w, tokenizer: tk, now: time.Now}
}

func (s *LedgerSyncer) Sync(ctx context.Context) error {
	file, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("opening assist-mem ledger: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat assist-mem ledger: %w", err)
	}
	syncedAt := s.now().UTC().Format("2006-01-02T15:04:05.000Z")
	scanner := bufio.NewScanner(file)
	// Ledger entries are small, but allow longer text/evidence fields than the
	// scanner default so a single explicit memory does not vanish.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var memories []db.Memory
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e ledgerEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			log.Printf("assist-mem sync: skipping line %d: malformed JSON: %v", lineNo, err)
			continue
		}
		m, ok := s.memoryFromEntry(e, info.ModTime().Unix(), syncedAt)
		if ok {
			memories = append(memories, m)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading assist-mem ledger: %w", err)
	}
	return s.writer.ReplaceMemoriesBySource(ctx, db.SourceAssistMem, memories)
}

func (s *LedgerSyncer) memoryFromEntry(e ledgerEntry, mtime int64, syncedAt string) (db.Memory, bool) {
	status := strings.TrimSpace(e.Status)
	if status == "" {
		status = "active"
	}
	if !strings.EqualFold(status, "active") || strings.TrimSpace(e.ID) == "" {
		return db.Memory{}, false
	}
	status = "active"
	body := renderLedgerBody(e)
	return db.Memory{
		RelPath:       filepath.ToSlash(filepath.Join("assist-mem", e.ID+".jsonl")),
		Source:        db.SourceAssistMem,
		Title:         ledgerTitle(e),
		Date:          ledgerDate(e.CreatedAt),
		ProblemType:   strings.TrimSpace(e.Source),
		Type:          strings.TrimSpace(e.Type),
		Status:        status,
		OriginSession: "assist-mem:" + e.ID,
		OriginProject: strings.TrimSpace(e.Project),
		Body:          body,
		BodyTokens:    s.tokenizer.Count(body),
		SourceMtime:   mtime,
		SyncedAt:      syncedAt,
	}, true
}

func ledgerDate(createdAt string) string {
	createdAt = strings.TrimSpace(createdAt)
	if len(createdAt) >= len("2006-01-02") {
		return createdAt[:len("2006-01-02")]
	}
	return createdAt
}

func ledgerTitle(e ledgerEntry) string {
	text := strings.TrimSpace(e.Text)
	if text == "" {
		return e.ID
	}
	const max = 96
	if len([]rune(text)) <= max {
		return text
	}
	r := []rune(text)
	return string(r[:max]) + "..."
}

func renderLedgerBody(e ledgerEntry) string {
	var b strings.Builder
	if text := strings.TrimSpace(e.Text); text != "" {
		b.WriteString(text)
		b.WriteString("\n")
	}
	if evidence := strings.TrimSpace(e.Evidence); evidence != "" {
		b.WriteString("\nEvidence: ")
		b.WriteString(evidence)
		b.WriteString("\n")
	}
	if len(e.Triggers) > 0 {
		b.WriteString("\nTriggers: ")
		b.WriteString(strings.Join(e.Triggers, ", "))
		b.WriteString("\n")
	}
	if scope := strings.TrimSpace(e.Scope); scope != "" {
		b.WriteString("\nScope: ")
		b.WriteString(scope)
		b.WriteString("\n")
	}
	return b.String()
}
