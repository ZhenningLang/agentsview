package db

// ComputeUsageEventFingerprint returns an exact ordered fingerprint for usage
// rows. Local callers set sanitize=true; PG callers fingerprint stored values.
func ComputeUsageEventFingerprint(events []UsageEvent, sanitize bool) string {
	h := sha256New()
	writeFingerprintInt(h, len(events))
	for _, original := range events {
		event := original
		if sanitize {
			_ = SanitizeUsageEvent(&event)
		}
		writeFingerprintBool(h, event.MessageOrdinal != nil)
		if event.MessageOrdinal != nil {
			writeFingerprintInt(h, *event.MessageOrdinal)
		} else {
			writeFingerprintInt(h, 0)
		}
		writeFingerprintString(h, event.Source)
		writeFingerprintString(h, event.Model)
		writeFingerprintInt(h, event.InputTokens)
		writeFingerprintInt(h, event.OutputTokens)
		writeFingerprintInt(h, event.CacheCreationInputTokens)
		writeFingerprintInt(h, event.CacheReadInputTokens)
		writeFingerprintInt(h, event.ReasoningTokens)
		writeFingerprintBool(h, event.CostUSD != nil)
		if event.CostUSD != nil {
			writeFingerprintString(h, formatFingerprintFloat(*event.CostUSD))
		} else {
			writeFingerprintString(h, "")
		}
		writeFingerprintString(h, event.CostStatus)
		writeFingerprintString(h, event.CostSource)
		writeFingerprintString(h, normalizeToolFingerprintTimestamp(event.OccurredAt))
		writeFingerprintString(h, event.DedupKey)
	}
	return fingerprintHex(h)
}
