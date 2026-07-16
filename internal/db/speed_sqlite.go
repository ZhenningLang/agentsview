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
	concurrency, err := db.speedConcurrency(ctx, q.Since, q.Until, q.BucketSec)
	if err != nil {
		return SpeedTrendResponse{}, err
	}
	BucketSpeedSamples(samples, q.BucketSec)
	return SpeedTrendResponse{
		BucketSec:   q.BucketSec,
		GroupBy:     q.GroupBy,
		Since:       q.Since,
		Until:       q.Until,
		Series:      AggregateSpeedTrend(samples, q.GroupBy),
		Concurrency: concurrency,
	}, nil
}

func (db *DB) GetSessionSpeed(
	ctx context.Context, sessionID string,
) (SessionSpeedResult, error) {
	rows, err := db.getReader().QueryContext(ctx, `
		WITH seq AS (
			SELECT m.session_id, m.ordinal, m.role, m.timestamp,
				m.output_tokens, m.has_output_tokens, m.model, s.agent,
				m.claude_request_id,
				LAG(m.timestamp) OVER (
					PARTITION BY m.session_id ORDER BY m.ordinal
				) AS prev_ts
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.session_id = ?
		)
		SELECT session_id, ordinal, role, COALESCE(timestamp, ''),
			output_tokens, has_output_tokens, COALESCE(model, ''), agent,
			COALESCE(claude_request_id, ''), COALESCE(prev_ts, '')
		FROM seq
		WHERE role = 'assistant'
		ORDER BY session_id, ordinal`, sessionID)
	if err != nil {
		return SessionSpeedResult{}, fmt.Errorf("querying session speed: %w", err)
	}
	defer rows.Close()
	messages, agent, err := scanSQLiteSpeedMessages(rows)
	if err != nil {
		return SessionSpeedResult{}, err
	}
	samples := SpeedEventsFromMessages(messages)
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
				m.claude_request_id,
				LAG(m.timestamp) OVER (
					PARTITION BY m.session_id ORDER BY m.ordinal
				) AS prev_ts
			FROM messages m
			JOIN sessions s ON s.id = m.session_id
			WHERE m.session_id IN (SELECT session_id FROM candidate_sessions)
		)
		SELECT session_id, ordinal, role, COALESCE(timestamp, ''),
			output_tokens, has_output_tokens, COALESCE(model, ''), agent,
			COALESCE(claude_request_id, ''), COALESCE(prev_ts, '')
		FROM seq
		WHERE role = 'assistant'
			AND julianday(timestamp) >= julianday(?)
			AND julianday(timestamp) < julianday(?)
		ORDER BY session_id, ordinal`,
		since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano),
		agent, agent,
		since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("querying speed samples: %w", err)
	}
	defer rows.Close()
	messages, _, err := scanSQLiteSpeedMessages(rows)
	if err != nil {
		return nil, err
	}
	return SpeedEventsFromMessages(messages), nil
}

func (db *DB) speedConcurrency(
	ctx context.Context, since, until time.Time, bucketSec int64,
) ([]SpeedConcurrencyPoint, error) {
	if bucketSec <= 0 {
		return []SpeedConcurrencyPoint{}, nil
	}
	rows, err := db.getReader().QueryContext(ctx, `
		SELECT
			CAST(strftime('%s', timestamp) AS INTEGER) / ? * ? AS bucket,
			COUNT(DISTINCT session_id) AS sessions
		FROM messages
		WHERE timestamp != ''
			AND julianday(timestamp) >= julianday(?)
			AND julianday(timestamp) < julianday(?)
		GROUP BY bucket
		ORDER BY bucket`,
		bucketSec, bucketSec,
		since.Format(time.RFC3339Nano), until.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("querying speed concurrency: %w", err)
	}
	defer rows.Close()
	return scanSpeedConcurrency(rows)
}

func scanSpeedConcurrency(rows *sql.Rows) ([]SpeedConcurrencyPoint, error) {
	points := make([]SpeedConcurrencyPoint, 0)
	for rows.Next() {
		var point SpeedConcurrencyPoint
		if err := rows.Scan(&point.T, &point.Sessions); err != nil {
			return nil, fmt.Errorf("scanning speed concurrency: %w", err)
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func scanSQLiteSpeedMessages(rows *sql.Rows) ([]SpeedMessage, string, error) {
	messages := make([]SpeedMessage, 0)
	agent := ""
	for rows.Next() {
		var current SpeedMessage
		var ordinal int
		var timestamp, prevTimestamp string
		var hasOutput int
		if err := rows.Scan(
			&current.SessionID, &ordinal, &current.Role, &timestamp,
			&current.OutputTokens, &hasOutput, &current.Model, &current.Agent,
			&current.RequestID, &prevTimestamp,
		); err != nil {
			return nil, "", fmt.Errorf("scanning speed sample: %w", err)
		}
		current.Ordinal = ordinal
		current.HasOutputTokens = hasOutput == 1
		current.Timestamp, current.TimestampValid = localTime(timestamp, time.UTC)
		current.PreviousTimestamp, current.PreviousTimestampValid =
			localTime(prevTimestamp, time.UTC)
		if agent == "" {
			agent = current.Agent
		}
		messages = append(messages, current)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return messages, agent, nil
}
