package db

// ComputeMessageDataFingerprint returns an exact, ordered fingerprint of all
// parser-owned message columns. Local callers set sanitize=true to compare the
// fixed-point shape that PostgreSQL push writes; PG callers use false to
// fingerprint the rows currently stored.
func ComputeMessageDataFingerprint(msgs []Message, sanitize bool) string {
	h := sha256New()
	writeFingerprintInt(h, len(msgs))
	for _, original := range msgs {
		msg := original
		if sanitize {
			_ = SanitizeMessage(&msg)
			msg.TokenUsage = []byte(SanitizeUTF8(string(msg.TokenUsage)))
		}
		writeFingerprintInt(h, msg.Ordinal)
		writeFingerprintString(h, msg.Role)
		writeFingerprintString(h, msg.Content)
		writeFingerprintString(h, msg.ThinkingText)
		writeFingerprintString(h, normalizeToolFingerprintTimestamp(msg.Timestamp))
		writeFingerprintBool(h, msg.HasThinking)
		writeFingerprintBool(h, msg.HasToolUse)
		writeFingerprintInt(h, msg.ContentLength)
		writeFingerprintBool(h, msg.IsSystem)
		writeFingerprintString(h, msg.Model)
		writeFingerprintString(h, string(msg.TokenUsage))
		writeFingerprintInt(h, msg.ContextTokens)
		writeFingerprintInt(h, msg.OutputTokens)
		writeFingerprintBool(h, msg.HasContextTokens)
		writeFingerprintBool(h, msg.HasOutputTokens)
		writeFingerprintString(h, msg.ClaudeMessageID)
		writeFingerprintString(h, msg.ClaudeRequestID)
		writeFingerprintString(h, msg.SourceType)
		writeFingerprintString(h, msg.SourceSubtype)
		writeFingerprintString(h, msg.SourceUUID)
		writeFingerprintString(h, msg.SourceParentUUID)
		writeFingerprintBool(h, msg.IsSidechain)
		writeFingerprintBool(h, msg.IsCompactBoundary)
	}
	return fingerprintHex(h)
}

func writeFingerprintBool(h fingerprintHash, value bool) {
	if value {
		writeFingerprintInt(h, 1)
		return
	}
	writeFingerprintInt(h, 0)
}
