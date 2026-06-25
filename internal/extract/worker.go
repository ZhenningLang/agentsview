package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

type LLMClient interface {
	ChatJSON(ctx context.Context, system, user string) (string, error)
}

type SessionStore interface {
	ListSessions(ctx context.Context, f db.SessionFilter) (db.SessionPage, error)
	GetAllMessages(ctx context.Context, sessionID string) ([]db.Message, error)
}

type Worker struct {
	Store  SessionStore
	LLM    LLMClient
	Writer Writer
	Audit  *AuditLog
	Limit  int

	now func() time.Time
}

func NewWorker(store SessionStore, llm LLMClient, root string, audit *AuditLog) *Worker {
	return &Worker{Store: store, LLM: llm, Writer: Writer{Root: root}, Audit: audit, Limit: 25, now: time.Now}
}

func (w *Worker) RunOnce(ctx context.Context) (RunRecord, error) {
	rec := RunRecord{StartedAt: w.clock().UTC().Format(time.RFC3339)}
	if w.Store == nil {
		rec.Skipped = true
		rec.Error = "session store unavailable"
		w.record(rec)
		return rec, nil
	}
	if w.LLM == nil {
		rec.Skipped = true
		rec.Error = "llm unavailable"
		w.record(rec)
		return rec, nil
	}
	page, err := w.Store.ListSessions(ctx, db.SessionFilter{Limit: w.limit()})
	if err != nil {
		rec.Error = fmt.Sprintf("listing sessions: %v", err)
		w.record(rec)
		return rec, nil
	}
	rec.SessionCount = len(page.Sessions)
	if len(page.Sessions) == 0 {
		rec.Skipped = true
		rec.Note = "no sessions"
		w.record(rec)
		return rec, nil
	}
	for _, sess := range page.Sessions {
		msgs, err := w.Store.GetAllMessages(ctx, sess.ID)
		if err != nil {
			rec.Candidates = append(rec.Candidates, CandidateRecord{Status: "error", Reason: fmt.Sprintf("messages %s: %v", sess.ID, err)})
			continue
		}
		if !hasPromptableMessages(msgs) {
			continue
		}
		raw, err := w.LLM.ChatJSON(ctx, systemPrompt, BuildUserPrompt(sess, msgs))
		if err != nil {
			rec.Error = fmt.Sprintf("llm: %v", err)
			rec.Candidates = append(rec.Candidates, CandidateRecord{Status: "error", Reason: fmt.Sprintf("llm %s: %v", sess.ID, err)})
			continue
		}
		items, err := ParseLLMResponse(raw)
		if err != nil {
			rec.Rejected++
			rec.Candidates = append(rec.Candidates, CandidateRecord{Status: "rejected", Reason: fmt.Sprintf("parsing llm response: %v", err)})
			continue
		}
		for _, item := range items {
			rec.CandidateCount++
			candidate, err := NewCandidate(item, sess, msgs, w.clock())
			if err != nil {
				rec.Rejected++
				rec.Candidates = append(rec.Candidates, CandidateRecord{Status: "rejected", Reason: err.Error()})
				continue
			}
			result, err := w.Writer.Write(ctx, candidate)
			if err != nil {
				rec.Error = fmt.Sprintf("writing candidate: %v", err)
				w.record(rec)
				return rec, nil
			}
			switch result.Status {
			case WriteWritten:
				rec.Written++
			case WriteDeduped:
				rec.Deduped++
			case WriteDriftRefused:
				rec.DriftRefused++
			case WriteGitignoreError:
				rec.Rejected++
			}
			rec.Candidates = append(rec.Candidates, CandidateRecord{CandidateID: result.CandidateID, Status: string(result.Status), Reason: result.Reason, Path: result.Path})
		}
	}
	rec.StagingFiles = countStagingFiles(w.Writer.Root)
	w.record(rec)
	return rec, nil
}

func countStagingFiles(root string) int {
	entries, err := os.ReadDir(RawDir(root))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	return count
}

func ParseLLMResponse(raw string) ([]LLMCandidate, error) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return nil, fmt.Errorf("no JSON object in LLM response")
	}
	var parsed struct {
		Candidates []LLMCandidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("empty candidates")
	}
	return parsed.Candidates, nil
}

func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func hasPromptableMessages(msgs []db.Message) bool {
	for _, msg := range msgs {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if (role == "user" || role == "assistant") && strings.TrimSpace(msg.Content) != "" {
			return true
		}
	}
	return false
}

func (w *Worker) limit() int {
	if w.Limit > 0 {
		return w.Limit
	}
	return 25
}

func (w *Worker) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *Worker) record(rec RunRecord) {
	rec.FinishedAt = w.clock().UTC().Format(time.RFC3339)
	if w.Audit != nil {
		_ = w.Audit.Append(rec)
	}
}
