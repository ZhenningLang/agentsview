package server_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/consolidate"
	"go.kenn.io/agentsview/internal/extract"
)

func TestMemoryQualityReturnsEmptyOnMissingFiles(t *testing.T) {
	dotfiles := t.TempDir()
	memoryDir := filepath.Join(dotfiles, "memory", "user")
	require.NoError(t, os.MkdirAll(memoryDir, 0o700))
	te := setup(t, withDotfilesRoot(dotfiles), withMemoryDir(memoryDir))

	w := te.get(t, "/api/v1/memory/quality")
	assertStatus(t, w, 200)
	body := decode[map[string]any](t, w)
	telemetry := body["telemetry"].(map[string]any)
	assert.Equal(t, float64(0), telemetry["capture_attempts"])
	assert.Empty(t, telemetry["scores"].([]any))
	assert.Empty(t, body["telemetry_rows"].([]any))
	extractSummary := body["extract"].(map[string]any)
	assert.NotNil(t, extractSummary["provider_usage"])
	consolidateSummary := body["consolidate"].(map[string]any)
	assert.NotNil(t, consolidateSummary["provider_usage"])
}

func TestMemoryQualityAggregatesTelemetryAndAudit(t *testing.T) {
	dotfiles := t.TempDir()
	memoryDir := filepath.Join(dotfiles, "memory", "user")
	require.NoError(t, os.MkdirAll(memoryDir, 0o700))
	te := setup(t, withDotfilesRoot(dotfiles), withMemoryDir(memoryDir))
	telemetry := filepath.Join(dotfiles, "memory_telemetry.jsonl")
	require.NoError(t, os.WriteFile(telemetry, []byte(`not-json
{"schema":"dotfiles.memory.telemetry.v1","ts":"2026-06-26T00:00:00Z","event":"capture","source":"memory_capture","status":"written","candidate_count":1,"candidate_written":true,"candidate_id":"candidate-1","duration_ms":3}
{"schema":"dotfiles.memory.telemetry.v1","ts":"2026-06-26T00:00:01Z","event":"capsule_route","source":"context_capsule","injected":true,"memory_injected":true,"capsule_count":1,"duration_ms":4}
{"schema":"dotfiles.memory.telemetry.v1","ts":"2026-06-26T00:00:02Z","event":"recall","source":"context_capsule","route":"lexical_fallback","hit_count":2,"scores":[0.7,0.5],"fallback_triggered":true,"duration_ms":5}
`), 0o600))
	extractAudit := extract.NewAuditLog(extract.AuditPath(te.dataDir))
	require.NoError(t, extractAudit.Append(extract.RunRecord{
		StartedAt:      "2026-06-26T00:01:00Z",
		SessionCount:   3,
		CandidateCount: 2,
		Written:        1,
		Deduped:        1,
		LLMDurationMS:  12,
		LLMCallCount:   2,
		ProviderUsage:  "extract",
		LLMUsage:       &extract.LLMUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
		LLMCost:        &extract.LLMCost{Currency: "USD", Amount: "0.1"},
	}))
	require.NoError(t, extractAudit.Append(extract.RunRecord{
		StartedAt:     "2026-06-26T00:01:30Z",
		LLMCallCount:  1,
		ProviderUsage: "extract",
		LLMCost:       &extract.LLMCost{Currency: "USD", Amount: "0.2"},
	}))
	consolidateAudit := consolidate.NewAuditLog(consolidate.AuditPath(memoryDir))
	require.NoError(t, consolidateAudit.Append(consolidate.RunRecord{
		StartedAt:      "2026-06-26T00:02:00Z",
		CandidateCount: 3,
		AddCount:       1,
		UpdateCount:    1,
		SkipCount:      1,
		Committed:      true,
		Resynced:       true,
		LLMDurationMS:  20,
		LLMCallCount:   1,
		ProviderUsage:  "consolidate",
		LLMUsage:       &consolidate.LLMUsage{PromptTokens: 12, CompletionTokens: 6, TotalTokens: 18},
		LLMCost:        &consolidate.LLMCost{Currency: "USD", Amount: "0.3"},
	}))
	require.NoError(t, consolidateAudit.Append(consolidate.RunRecord{
		StartedAt:     "2026-06-26T00:02:30Z",
		LLMCallCount:  1,
		ProviderUsage: "consolidate",
		LLMCost:       &consolidate.LLMCost{Currency: "USD", Amount: "0.4"},
	}))

	w := te.get(t, "/api/v1/memory/quality?limit=10")
	assertStatus(t, w, 200)
	body := decode[map[string]any](t, w)
	qualityTelemetry := body["telemetry"].(map[string]any)
	assert.Equal(t, float64(1), qualityTelemetry["capture_attempts"])
	assert.Equal(t, float64(1), qualityTelemetry["capture_written"])
	assert.Equal(t, float64(1), qualityTelemetry["injection_count"])
	assert.Equal(t, float64(1), qualityTelemetry["recall_count"])
	assert.Equal(t, float64(2), qualityTelemetry["recall_hit_count"])
	assert.Equal(t, float64(1), qualityTelemetry["fallback_count"])
	assert.Len(t, qualityTelemetry["scores"].([]any), 2)
	assert.Len(t, body["telemetry_rows"].([]any), 3)
	extractSummary := body["extract"].(map[string]any)
	assert.Equal(t, float64(3), extractSummary["sessions_scanned"])
	assert.Equal(t, float64(2), extractSummary["candidate_count"])
	assert.Equal(t, float64(3), extractSummary["llm_call_count"])
	assert.Equal(t, float64(12), extractSummary["llm_duration_ms"])
	assert.Equal(t, float64(2), extractSummary["provider_usage"].(map[string]any)["extract"])
	assert.Equal(t, float64(14), extractSummary["llm_usage"].(map[string]any)["total_tokens"])
	assert.Equal(t, "0.3", extractSummary["llm_cost"].(map[string]any)["amount"])
	consolidateSummary := body["consolidate"].(map[string]any)
	assert.Equal(t, float64(3), consolidateSummary["candidate_count"])
	assert.Equal(t, float64(1), consolidateSummary["add_count"])
	assert.Equal(t, float64(1), consolidateSummary["update_count"])
	assert.Equal(t, float64(1), consolidateSummary["skip_count"])
	assert.Equal(t, float64(1), consolidateSummary["committed"])
	assert.Equal(t, float64(18), consolidateSummary["llm_usage"].(map[string]any)["total_tokens"])
	assert.Equal(t, "0.7", consolidateSummary["llm_cost"].(map[string]any)["amount"])
}
