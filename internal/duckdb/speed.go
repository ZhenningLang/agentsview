package duckdb

import (
	"context"
	"fmt"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Store) GetSpeedTrend(ctx context.Context, q db.SpeedTrendQuery) (db.SpeedTrendResponse, error) {
	samples, err := s.speedSamples(ctx, q.Agent, q.Since, q.Until)
	if err != nil {
		return db.SpeedTrendResponse{}, err
	}
	db.BucketSpeedSamples(samples, q.BucketSec)
	return db.SpeedTrendResponse{BucketSec: q.BucketSec, GroupBy: q.GroupBy, Since: q.Since, Until: q.Until, Series: db.AggregateSpeedTrend(samples, q.GroupBy)}, nil
}

func (s *Store) GetSessionSpeed(ctx context.Context, sessionID string) (db.SessionSpeedResult, error) {
	rows, err := s.duck.QueryContext(ctx, `
		WITH seq AS (
			SELECT m.session_id, m.ordinal, m.role, m.timestamp, m.output_tokens,
				m.has_output_tokens, m.model, s.agent,
				LAG(m.timestamp) OVER (PARTITION BY m.session_id ORDER BY m.ordinal) AS prev_ts
			FROM messages m JOIN sessions s ON s.id = m.session_id WHERE m.session_id = ?
		)
		SELECT session_id, ordinal, role, timestamp, output_tokens, has_output_tokens,
			COALESCE(model, ''), agent, prev_ts FROM seq WHERE role = 'assistant'`, sessionID)
	if err != nil {
		return db.SessionSpeedResult{}, fmt.Errorf("querying duckdb session speed: %w", err)
	}
	defer rows.Close()
	samples, agent, err := scanDuckSpeedRows(rows)
	if err != nil {
		return db.SessionSpeedResult{}, err
	}
	return db.SessionSpeedResult{Agent: agent, Speed: db.SessionSpeedFromSamples(samples)}, nil
}

func (s *Store) GetSpeedBaselineSessions(ctx context.Context, agent string, since, until time.Time) ([]db.SpeedSessionRate, error) {
	samples, err := s.speedSamples(ctx, agent, since, until)
	if err != nil {
		return nil, err
	}
	return db.SpeedSessionRatesFromSamples(samples), nil
}

func (s *Store) speedSamples(ctx context.Context, agent string, since, until time.Time) ([]db.SpeedSample, error) {
	rows, err := s.duck.QueryContext(ctx, `
		WITH candidate_sessions AS (
			SELECT DISTINCT m.session_id FROM messages m JOIN sessions s ON s.id = m.session_id
			WHERE m.role = 'assistant' AND m.timestamp >= ? AND m.timestamp < ?
				AND (? = '' OR s.agent = ?)
		), seq AS (
			SELECT m.session_id, m.ordinal, m.role, m.timestamp, m.output_tokens,
				m.has_output_tokens, m.model, s.agent,
				LAG(m.timestamp) OVER (PARTITION BY m.session_id ORDER BY m.ordinal) AS prev_ts
			FROM messages m JOIN sessions s ON s.id = m.session_id
			WHERE m.session_id IN (SELECT session_id FROM candidate_sessions)
		)
		SELECT session_id, ordinal, role, timestamp, output_tokens, has_output_tokens,
			COALESCE(model, ''), agent, prev_ts FROM seq
		WHERE role = 'assistant' AND timestamp >= ? AND timestamp < ?`,
		since, until, agent, agent, since, until)
	if err != nil {
		return nil, fmt.Errorf("querying duckdb speed samples: %w", err)
	}
	defer rows.Close()
	samples, _, err := scanDuckSpeedRows(rows)
	return samples, err
}

func scanDuckSpeedRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]db.SpeedSample, string, error) {
	samples := make([]db.SpeedSample, 0)
	agent := ""
	for rows.Next() {
		var current db.SpeedMessage
		var timestamp, prevTimestamp any
		if err := rows.Scan(&current.SessionID, &current.Ordinal, &current.Role, &timestamp, &current.OutputTokens, &current.HasOutputTokens, &current.Model, &current.Agent, &prevTimestamp); err != nil {
			return nil, "", fmt.Errorf("scanning duckdb speed sample: %w", err)
		}
		if parsed, ok := parseTimestamp(formatDBTime(timestamp)); ok {
			current.Timestamp, current.TimestampValid = parsed, true
		}
		if parsed, ok := parseTimestamp(formatDBTime(prevTimestamp)); ok {
			current.PreviousTimestamp, current.PreviousTimestampValid = parsed, true
		}
		if agent == "" {
			agent = current.Agent
		}
		if sample, ok := db.NewSpeedSampleWithPreviousTimestamp(current); ok {
			samples = append(samples, sample)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return samples, agent, nil
}
