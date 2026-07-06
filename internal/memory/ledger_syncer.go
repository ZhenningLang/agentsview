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
	embedder  Embedder
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
	Topic     string   `json:"topic"`
	Triggers  []string `json:"triggers"`
	Type      string   `json:"type"`
}

type ledgerCandidate struct {
	entry     ledgerEntry
	lineNo    int
	createdAt time.Time
	hasTime   bool
}

func NewLedgerSyncer(path string, w Writer, tk skills.Tokenizer) *LedgerSyncer {
	if tk == nil {
		tk = skills.NewHeuristicTokenizer()
	}
	return &LedgerSyncer{path: path, writer: w, tokenizer: tk, now: time.Now}
}

// NewLedgerSyncerWithEmbedder mirrors explicit assist-mem ledger rows while
// optionally persisting embeddings for cross-source synthesis. A nil embedder
// keeps the lexical-only sync path unchanged.
func NewLedgerSyncerWithEmbedder(path string, w Writer, tk skills.Tokenizer, e Embedder) *LedgerSyncer {
	s := NewLedgerSyncer(path, w, tk)
	s.embedder = e
	return s
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
	latestByTopic := map[string]ledgerCandidate{}
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
		candidate, ok := ledgerCandidateFromEntry(e, lineNo)
		if !ok {
			continue
		}
		key := ledgerTopicKey(candidate.entry)
		if current, exists := latestByTopic[key]; !exists || candidate.isNewerThan(current) {
			latestByTopic[key] = candidate
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading assist-mem ledger: %w", err)
	}
	memories := make([]db.Memory, 0, len(latestByTopic))
	previous := loadPreviousEmbeddings(ctx, s.writer, db.SourceAssistMem)
	for _, candidate := range latestByTopic {
		m, ok := s.memoryFromEntry(candidate.entry, info.ModTime().Unix(), syncedAt)
		if ok {
			if err := populateMemoryEmbedding(ctx, s.embedder, &m, previous); err != nil {
				return err
			}
			memories = append(memories, m)
		}
	}
	return s.writer.ReplaceMemoriesBySource(ctx, db.SourceAssistMem, memories)
}

func ledgerCandidateFromEntry(e ledgerEntry, lineNo int) (ledgerCandidate, bool) {
	status := strings.TrimSpace(e.Status)
	if status == "" {
		status = "active"
	}
	if !strings.EqualFold(status, "active") || strings.TrimSpace(e.ID) == "" {
		return ledgerCandidate{}, false
	}
	createdAt, err := time.Parse(time.RFC3339, strings.TrimSpace(e.CreatedAt))
	return ledgerCandidate{entry: e, lineNo: lineNo, createdAt: createdAt, hasTime: err == nil}, true
}

func (c ledgerCandidate) isNewerThan(other ledgerCandidate) bool {
	if c.hasTime && other.hasTime && !c.createdAt.Equal(other.createdAt) {
		return c.createdAt.After(other.createdAt)
	}
	return c.lineNo > other.lineNo
}

func ledgerTopicKey(e ledgerEntry) string {
	topic := strings.TrimSpace(e.Topic)
	if topic == "" {
		return "id:" + strings.TrimSpace(e.ID)
	}
	return strings.Join([]string{
		"topic",
		strings.TrimSpace(e.Project),
		strings.TrimSpace(e.Scope),
		topic,
	}, "\x00")
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
	if createdAt == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		loc := time.FixedZone("Asia/Shanghai", 8*60*60)
		return t.In(loc).Format("2006-01-02 15:04:05")
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
