package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	EnrichStatusOK              = "ok"
	EnrichStatusError           = "error"
	EnrichStatusNoContent       = "no_content"
	EnrichStatusSkippedTooShort = "skipped_too_short"
)

type EnrichmentStatusReport struct {
	Total           int            `json:"total"`
	Enriched        int            `json:"enriched"`
	Pending         int            `json:"pending"`
	SkippedTooShort int            `json:"skipped_too_short"`
	NoContent       int            `json:"no_content"`
	Errors          int            `json:"errors"`
	ByStatus        map[string]int `json:"by_status"`
}

type EnrichCandidateOptions struct {
	Project             string
	Force               bool
	Limit               int
	MinUserMessages     int
	ReenrichMsgDelta    int
	ReenrichIdleMinutes int
	Now                 time.Time
}

type EnrichCandidate struct {
	ID               string
	Project          string
	Agent            string
	FirstMessage     string
	MessageCount     int
	UserMessageCount int
	EnrichStatus     string
}

type EnrichmentWrite struct {
	Title        string
	Summary      string
	Keywords     []string
	Model        string
	Status       string
	Error        string
	MessageCnt   int
	EnrichedAt   time.Time
	Embedding    []float32
	HasEmbedding bool
}

func (db *DB) EnrichCandidates(
	ctx context.Context,
	opts EnrichCandidateOptions,
) ([]EnrichCandidate, error) {
	opts = normalizeEnrichCandidateOptions(opts)
	args := []any{opts.MinUserMessages}
	where := []string{
		"s.deleted_at IS NULL",
		"s.user_message_count >= ?",
	}
	if opts.Project != "" {
		where = append(where, "s.project = ?")
		args = append(args, opts.Project)
	}
	if !opts.Force {
		where = append(where, `(
			COALESCE(s.enrich_status, '') IN ('', 'error', 'skipped_too_short', 'no_content')
			OR (s.enriched_msg_count > 0 AND s.message_count - s.enriched_msg_count >= ?)
		)`)
		args = append(args, opts.ReenrichMsgDelta)
		where = append(where, `(
			COALESCE(s.ended_at, '') != ''
			OR datetime(COALESCE((
				SELECT MAX(m.timestamp)
				FROM messages m
				WHERE m.session_id = s.id AND COALESCE(m.timestamp, '') != ''
			), COALESCE(s.started_at, s.created_at, ''))) <= datetime(?)
		)`)
		args = append(args, opts.Now.Add(-time.Duration(opts.ReenrichIdleMinutes)*time.Minute).Format(time.RFC3339))
	}
	query := fmt.Sprintf(`
		SELECT s.id, s.project, s.agent, COALESCE(s.first_message, ''),
		       s.message_count, s.user_message_count, COALESCE(s.enrich_status, '')
		FROM sessions s
		WHERE %s
		ORDER BY COALESCE(s.ended_at, s.started_at, s.created_at, '') DESC, s.id DESC`, strings.Join(where, " AND "))
	if opts.Limit > 0 {
		query += "\n\t\tLIMIT ?"
		args = append(args, opts.Limit)
	}
	rows, err := db.getReader().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query enrichment candidates: %w", err)
	}
	defer rows.Close()
	var out []EnrichCandidate
	for rows.Next() {
		var c EnrichCandidate
		if err := rows.Scan(
			&c.ID, &c.Project, &c.Agent, &c.FirstMessage,
			&c.MessageCount, &c.UserMessageCount, &c.EnrichStatus,
		); err != nil {
			return nil, fmt.Errorf("scan enrichment candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enrichment candidates: %w", err)
	}
	return out, nil
}

func (db *DB) MarkEnrichmentSkippedTooShort(
	ctx context.Context,
	opts EnrichCandidateOptions,
) (int, error) {
	opts = normalizeEnrichCandidateOptions(opts)
	args := []any{opts.MinUserMessages}
	where := []string{
		"deleted_at IS NULL",
		"user_message_count < ?",
		"COALESCE(enrich_status, '') IN ('', 'error', 'no_content')",
	}
	if opts.Project != "" {
		where = append(where, "project = ?")
		args = append(args, opts.Project)
	}
	query := fmt.Sprintf(`
		UPDATE sessions
		SET enrich_status = 'skipped_too_short',
		    enrich_error = '',
		    local_modified_at = strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now')
		WHERE %s`, strings.Join(where, " AND "))
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.getWriter().ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("mark enrichment skipped too short: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (db *DB) WriteEnrichment(
	ctx context.Context,
	sessionID string,
	write EnrichmentWrite,
) error {
	status := strings.TrimSpace(write.Status)
	if status == "" {
		status = EnrichStatusOK
	}
	if write.EnrichedAt.IsZero() {
		write.EnrichedAt = time.Now().UTC()
	}
	enrichedAt := write.EnrichedAt.UTC().Format(time.RFC3339Nano)
	db.mu.Lock()
	defer db.mu.Unlock()
	switch status {
	case EnrichStatusOK:
		embeddingSQL := ""
		args := []any{
			strings.TrimSpace(write.Title), strings.TrimSpace(write.Summary),
			joinKeywords(write.Keywords), enrichedAt, write.MessageCnt,
			strings.TrimSpace(write.Model),
		}
		if write.HasEmbedding {
			encoded, err := EncodeEmbedding(write.Embedding)
			if err != nil {
				return fmt.Errorf("write enrichment embedding %s: %w", sessionID, err)
			}
			embeddingSQL = "llm_embedding = ?,\n\t\t\t    llm_embedding_dim = ?,"
			args = append(args, encoded, len(write.Embedding))
		}
		args = append(args, sessionID)
		res, err := db.getWriter().ExecContext(ctx, `
			UPDATE sessions
			SET llm_title = ?,
			    llm_summary = ?,
			    llm_keywords = ?,
			    enriched_at = ?,
			    enriched_msg_count = ?,
			    enrich_model = ?,
			    enrich_status = 'ok',
			    enrich_error = '',
			    `+embeddingSQL+`
			    local_modified_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ?`,
			args...)
		if err != nil {
			return fmt.Errorf("write enrichment success %s: %w", sessionID, err)
		}
		return requireRowsAffected(res, "write enrichment success", sessionID)
	case EnrichStatusError, EnrichStatusNoContent:
		res, err := db.getWriter().ExecContext(ctx, `
			UPDATE sessions
			SET enrich_status = ?,
			    enrich_error = ?,
			    local_modified_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ?`, status, strings.TrimSpace(write.Error), sessionID)
		if err != nil {
			return fmt.Errorf("write enrichment status %s for %s: %w", status, sessionID, err)
		}
		return requireRowsAffected(res, "write enrichment status", sessionID)
	case EnrichStatusSkippedTooShort:
		// Keep a single-session path for tests and future manual repair flows;
		// batch candidate evaluation uses MarkEnrichmentSkippedTooShort.
		res, err := db.getWriter().ExecContext(ctx, `
			UPDATE sessions
			SET enrich_status = 'skipped_too_short',
			    enrich_error = '',
			    local_modified_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id = ?`, sessionID)
		if err != nil {
			return fmt.Errorf("write enrichment skipped %s: %w", sessionID, err)
		}
		return requireRowsAffected(res, "write enrichment skipped", sessionID)
	default:
		return fmt.Errorf("write enrichment %s: unsupported status %q", sessionID, status)
	}
}

func (db *DB) GetEnrichmentStatus(ctx context.Context) (EnrichmentStatusReport, error) {
	rows, err := db.getReader().QueryContext(ctx, `
		SELECT COALESCE(enrich_status, ''), COUNT(*)
		FROM sessions
		WHERE deleted_at IS NULL
		GROUP BY COALESCE(enrich_status, '')`)
	if err != nil {
		return EnrichmentStatusReport{}, fmt.Errorf("query enrichment status: %w", err)
	}
	defer rows.Close()
	report := EnrichmentStatusReport{ByStatus: map[string]int{}}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return EnrichmentStatusReport{}, fmt.Errorf("scan enrichment status: %w", err)
		}
		AccumulateEnrichmentStatus(&report, status, count)
	}
	if err := rows.Err(); err != nil {
		return EnrichmentStatusReport{}, fmt.Errorf("iterate enrichment status: %w", err)
	}
	return report, nil
}

func AccumulateEnrichmentStatus(report *EnrichmentStatusReport, status string, count int) {
	if report.ByStatus == nil {
		report.ByStatus = map[string]int{}
	}
	report.Total += count
	report.ByStatus[status] = count
	switch status {
	case "":
		report.Pending += count
	case EnrichStatusOK:
		report.Enriched += count
	case EnrichStatusSkippedTooShort:
		report.SkippedTooShort += count
	case EnrichStatusNoContent:
		report.NoContent += count
	case EnrichStatusError:
		report.Errors += count
	}
}

func normalizeEnrichCandidateOptions(opts EnrichCandidateOptions) EnrichCandidateOptions {
	if opts.MinUserMessages <= 0 {
		opts.MinUserMessages = 3
	}
	if opts.ReenrichMsgDelta <= 0 {
		opts.ReenrichMsgDelta = 20
	}
	if opts.ReenrichIdleMinutes <= 0 {
		opts.ReenrichIdleMinutes = 30
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	return opts
}

func joinKeywords(keywords []string) string {
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" {
			out = append(out, keyword)
		}
	}
	return strings.Join(out, ",")
}

func requireRowsAffected(res sql.Result, op, sessionID string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s %s: checking rows affected: %w", op, sessionID, err)
	}
	if n == 0 {
		return fmt.Errorf("%s %s: session not found", op, sessionID)
	}
	return nil
}
