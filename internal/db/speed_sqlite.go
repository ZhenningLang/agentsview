package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (db *DB) GetSpeedTrend(
	ctx context.Context, q SpeedTrendQuery,
) (SpeedTrendResponse, error) {
	samples, err := db.speedSamples(ctx, q.Agent, q.Since, q.Until)
	if err != nil {
		return SpeedTrendResponse{}, err
	}
	BucketSpeedSamples(samples, q.BucketSec)
	return SpeedTrendResponse{
		BucketSec: q.BucketSec,
		GroupBy:   q.GroupBy,
		Since:     q.Since,
		Until:     q.Until,
		Series:    AggregateSpeedTrend(samples, q.GroupBy),
	}, nil
}

func (db *DB) GetSessionSpeed(
	ctx context.Context, sessionID string,
) (SessionSpeedResult, error) {
	rows, err := db.getReader().QueryContext(ctx, `
		WITH seq AS (
			SELECT m.session_id, m.ordinal, m.role, m.timestamp,
				m.output_tokens, m.has_output_tokens, m.model, s.agent,
				LAG(m.timestamp) OVER (
					PARTITION BY m.session_id ORDER BY m.ordinal
				) AS prev_ts
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.session_id = ?
		)
		SELECT session_id, ordinal, role, COALESCE(timestamp, ''),
			output_tokens, has_output_tokens, COALESCE(model, ''), agent,
			COALESCE(prev_ts, '')
		FROM seq
		WHERE role = 'assistant'`, sessionID)
	if err != nil {
		return SessionSpeedResult{}, fmt.Errorf("querying session speed: %w", err)
	}
	defer rows.Close()
	samples, agent, err := scanSQLiteSpeedSamples(rows)
	if err != nil {
		return SessionSpeedResult{}, err
	}
	return SessionSpeedResult{Agent: agent, Speed: SessionSpeedFromSamples(samples)}, nil
}

func (db *DB) GetSpeedBaselineSessions(
	ctx context.Context, agent string, since, until time.Time,
) ([]SpeedSessionRate, error) {
	samples, err := db.speedSamples(ctx, agent, since, until)
	if err != nil {
		return nil, err
	}
	return SpeedSessionRatesFromSamples(samples), nil
}

func (db *DB) speedSamples(
	ctx context.Context, agent string, since, until time.Time,
) ([]SpeedSample, error) {
	rows, err := db.getReader().QueryContext(ctx, `
		WITH candidate_sessions AS (
			SELECT DISTINCT m.session_id
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.role = 'assistant'
				AND julianday(m.timestamp) >= julianday(?)
				AND julianday(m.timestamp) < julianday(?)
				AND (? = '' OR s.agent = ?)
		), seq AS (
			SELECT m.session_id, m.ordinal, m.role, m.timestamp,
				m.output_tokens, m.has_output_tokens, m.model, s.agent,
				LAG(m.timestamp) OVER (
					PARTITION BY m.session_id ORDER BY m.ordinal
				) AS prev_ts
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.session_id IN (SELECT session_id FROM candidate_sessions)
		)
		SELECT session_id, ordinal, role, COALESCE(timestamp, ''),
			output_tokens, has_output_tokens, COALESCE(model, ''), agent,
			COALESCE(prev_ts, '')
		FROM seq
		WHERE role = 'assistant'
			AND julianday(timestamp) >= julianday(?)
			AND julianday(timestamp) < julianday(?)`,
		since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano),
		agent, agent,
		since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("querying speed samples: %w", err)
	}
	defer rows.Close()
	samples, _, err := scanSQLiteSpeedSamples(rows)
	return samples, err
}

func scanSQLiteSpeedSamples(rows *sql.Rows) ([]SpeedSample, string, error) {
	samples := make([]SpeedSample, 0)
	agent := ""
	for rows.Next() {
		var current, previous SpeedMessage
		var ordinal int
		var timestamp, prevTimestamp string
		var hasOutput int
		if err := rows.Scan(
			&current.SessionID, &ordinal, &current.Role, &timestamp,
			&current.OutputTokens, &hasOutput, &current.Model, &current.Agent,
			&prevTimestamp,
		); err != nil {
			return nil, "", fmt.Errorf("scanning speed sample: %w", err)
		}
		current.Ordinal = ordinal
		current.HasOutputTokens = hasOutput == 1
		current.Timestamp, current.TimestampValid = localTime(timestamp, time.UTC)
		previous.Timestamp, previous.TimestampValid = localTime(prevTimestamp, time.UTC)
		if agent == "" {
			agent = current.Agent
		}
		current.PreviousTimestamp = previous.Timestamp
		current.PreviousTimestampValid = previous.TimestampValid
		if sample, ok := NewSpeedSampleWithPreviousTimestamp(current); ok {
			samples = append(samples, sample)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return samples, agent, nil
}
