package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeUsageEventFingerprintUsesMicrosecondFixedPoint(t *testing.T) {
	cost := 0.25
	base := UsageEvent{
		Source: "generation", Model: "model",
		InputTokens: 1, OutputTokens: 2,
		CacheCreationInputTokens: 3, CacheReadInputTokens: 4,
		ReasoningTokens: 5, CostUSD: &cost,
		CostStatus: "estimated", CostSource: "catalog",
		OccurredAt: "2026-07-13T12:00:00.123456789Z",
		DedupKey:   "key",
	}
	clean := base
	clean.OccurredAt = "2026-07-13T12:00:00.123456Z"
	want := ComputeUsageEventFingerprint([]UsageEvent{clean}, true)
	assert.Equal(t, want,
		ComputeUsageEventFingerprint([]UsageEvent{base}, true))

	dirty := base
	dirty.Source = "generation\x07"
	dirty.Model = "mo\x00del"
	dirty.InputTokens = 9_000_000
	dirty.OutputTokens = -1
	clean.Source = "generation"
	clean.Model = "model"
	clean.InputTokens = MaxPlausibleTokens
	clean.OutputTokens = 0
	want = ComputeUsageEventFingerprint([]UsageEvent{clean}, true)
	assert.Equal(t, want,
		ComputeUsageEventFingerprint([]UsageEvent{dirty}, true))
	assert.NotEqual(t, want,
		ComputeUsageEventFingerprint([]UsageEvent{dirty}, false))
}
