package db

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strconv"
	"time"
)

type fingerprintHash = hash.Hash

func sha256New() fingerprintHash { return sha256.New() }

func fingerprintHex(h fingerprintHash) string {
	return hex.EncodeToString(h.Sum(nil))
}

func formatFingerprintFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// ToolCallFingerprintRow is the backend-neutral shape used to compare nested
// tool data between SQLite and PostgreSQL.
type ToolCallFingerprintRow struct {
	MessageOrdinal int
	CallIndex      int
	Call           ToolCall
}

// ToolResultEventFingerprintRow is the backend-neutral event shape used by
// ToolDataFingerprint.
type ToolResultEventFingerprintRow struct {
	MessageOrdinal int
	CallIndex      int
	Event          ToolResultEvent
}

// ComputeToolDataFingerprint returns an exact, order-sensitive fingerprint.
// Local callers set sanitize=true to fingerprint the shape that PG push will
// write; PG callers use false to fingerprint the bytes currently stored.
func ComputeToolDataFingerprint(
	calls []ToolCallFingerprintRow,
	events []ToolResultEventFingerprintRow,
	sanitize bool,
) string {
	h := sha256New()
	writeFingerprintInt(h, len(calls))
	for _, row := range calls {
		call := row.Call
		if sanitize {
			_ = sanitizeToolCall(&call)
		}
		writeFingerprintInt(h, row.MessageOrdinal)
		writeFingerprintInt(h, row.CallIndex)
		writeFingerprintString(h, call.ToolName)
		writeFingerprintString(h, call.Category)
		writeFingerprintString(h, call.ToolUseID)
		writeFingerprintString(h, call.InputJSON)
		writeFingerprintString(h, call.SkillName)
		writeFingerprintInt(h, call.ResultContentLength)
		writeFingerprintString(h, call.ResultContent)
		writeFingerprintString(h, call.SubagentSessionID)
	}
	writeFingerprintInt(h, len(events))
	for _, row := range events {
		event := row.Event
		if sanitize {
			_ = sanitizeToolResultEvent(&event)
		}
		writeFingerprintInt(h, row.MessageOrdinal)
		writeFingerprintInt(h, row.CallIndex)
		writeFingerprintInt(h, event.EventIndex)
		writeFingerprintString(h, event.ToolUseID)
		writeFingerprintString(h, event.AgentID)
		writeFingerprintString(h, event.SubagentSessionID)
		writeFingerprintString(h, event.Source)
		writeFingerprintString(h, event.Status)
		writeFingerprintString(h, event.Content)
		writeFingerprintInt(h, event.ContentLength)
		writeFingerprintString(h, normalizeToolFingerprintTimestamp(event.Timestamp))
	}
	return fingerprintHex(h)
}

func normalizeToolFingerprintTimestamp(value string) string {
	parsed, ok := ParseStoredTimestamp(value)
	if !ok {
		return ""
	}
	return parsed.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
}

func writeFingerprintInt(h fingerprintHash, value int) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(int64(value)))
	_, _ = h.Write(buf[:])
}

func writeFingerprintString(h fingerprintHash, value string) {
	writeFingerprintInt(h, len(value))
	_, _ = h.Write([]byte(value))
}
