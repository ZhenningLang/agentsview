package memoryquality

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.kenn.io/agentsview/internal/consolidate"
	"go.kenn.io/agentsview/internal/extract"
)

type TelemetryRecord struct {
	Schema         string         `json:"schema"`
	TS             string         `json:"ts"`
	Event          string         `json:"event"`
	Source         string         `json:"source"`
	DurationMS     int            `json:"duration_ms,omitempty"`
	Status         string         `json:"status,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	CandidateCount int            `json:"candidate_count,omitempty"`
	CandidateWrite bool           `json:"candidate_written,omitempty"`
	CandidateID    string         `json:"candidate_id,omitempty"`
	Platform       string         `json:"platform,omitempty"`
	SkippedReasons map[string]int `json:"skipped_reasons,omitempty"`
	CapsuleCount   int            `json:"capsule_count,omitempty"`
	Injected       bool           `json:"injected,omitempty"`
	MemoryInjected bool           `json:"memory_injected,omitempty"`
	PromptChars    int            `json:"prompt_chars,omitempty"`
	ContextChars   int            `json:"context_chars,omitempty"`
	Route          string         `json:"route,omitempty"`
	HitCount       int            `json:"hit_count,omitempty"`
	Scores         []float64      `json:"scores,omitempty"`
	Fallback       bool           `json:"fallback_triggered,omitempty"`
	FallbackReason string         `json:"fallback_reason,omitempty"`
}

type TelemetrySummary struct {
	CaptureAttempts int       `json:"capture_attempts"`
	CandidateCount  int       `json:"candidate_count"`
	CaptureWritten  int       `json:"capture_written"`
	InjectionCount  int       `json:"injection_count"`
	RecallCount     int       `json:"recall_count"`
	RecallHitCount  int       `json:"recall_hit_count"`
	FallbackCount   int       `json:"fallback_count"`
	Scores          []float64 `json:"scores"`
}

type ExtractSummary struct {
	SessionsScanned int               `json:"sessions_scanned"`
	CandidateCount  int               `json:"candidate_count"`
	Written         int               `json:"written"`
	Deduped         int               `json:"deduped"`
	Rejected        int               `json:"rejected"`
	DriftRefused    int               `json:"drift_refused"`
	LLMDurationMS   int               `json:"llm_duration_ms"`
	LLMCallCount    int               `json:"llm_call_count"`
	ProviderUsage   map[string]int    `json:"provider_usage"`
	LLMUsage        *extract.LLMUsage `json:"llm_usage,omitempty"`
	LLMCost         *extract.LLMCost  `json:"llm_cost,omitempty"`
}

type ConsolidateSummary struct {
	CandidateCount int                   `json:"candidate_count"`
	AddCount       int                   `json:"add_count"`
	UpdateCount    int                   `json:"update_count"`
	SkipCount      int                   `json:"skip_count"`
	Committed      int                   `json:"committed"`
	Resynced       int                   `json:"resynced"`
	LLMDurationMS  int                   `json:"llm_duration_ms"`
	LLMCallCount   int                   `json:"llm_call_count"`
	ProviderUsage  map[string]int        `json:"provider_usage"`
	LLMUsage       *consolidate.LLMUsage `json:"llm_usage,omitempty"`
	LLMCost        *consolidate.LLMCost  `json:"llm_cost,omitempty"`
}

type QualityResponse struct {
	Telemetry     TelemetrySummary   `json:"telemetry"`
	Extract       ExtractSummary     `json:"extract"`
	Consolidate   ConsolidateSummary `json:"consolidate"`
	TelemetryRows []TelemetryRecord  `json:"telemetry_rows"`
}

func TelemetryPath(dotfilesRoot string) string {
	if override := strings.TrimSpace(os.Getenv("DOTFILES_MEMORY_TELEMETRY_PATH")); override != "" {
		return override
	}
	if strings.TrimSpace(dotfilesRoot) == "" {
		return ""
	}
	return filepath.Join(dotfilesRoot, "memory_telemetry.jsonl")
}

func ReadTelemetry(path string, limit int) ([]TelemetryRecord, error) {
	if path == "" {
		return []TelemetryRecord{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []TelemetryRecord{}, nil
		}
		return nil, err
	}
	defer f.Close()
	var rows []TelemetryRecord
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for s.Scan() {
		var rec TelemetryRecord
		if err := json.Unmarshal(s.Bytes(), &rec); err != nil {
			continue
		}
		rows = append(rows, rec)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	reverse(rows)
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	if rows == nil {
		rows = []TelemetryRecord{}
	}
	return rows, nil
}

func BuildQualityResponse(dotfilesRoot, dataDir, memoryDir string, limit int) (QualityResponse, error) {
	telemetryRows, err := ReadTelemetry(TelemetryPath(dotfilesRoot), limit)
	if err != nil {
		return QualityResponse{}, err
	}
	telemetry := summarizeTelemetry(telemetryRows)
	extractPath := ""
	if strings.TrimSpace(dataDir) != "" {
		extractPath = extract.AuditPath(dataDir)
	}
	extractSummary, err := summarizeExtract(extractPath, limit)
	if err != nil {
		return QualityResponse{}, err
	}
	consolidatePath := ""
	if strings.TrimSpace(memoryDir) != "" {
		consolidatePath = consolidate.AuditPath(memoryDir)
	}
	consolidateSummary, err := summarizeConsolidate(consolidatePath, limit)
	if err != nil {
		return QualityResponse{}, err
	}
	return QualityResponse{
		Telemetry:     telemetry,
		Extract:       extractSummary,
		Consolidate:   consolidateSummary,
		TelemetryRows: telemetryRows,
	}, nil
}

func summarizeTelemetry(rows []TelemetryRecord) TelemetrySummary {
	var out TelemetrySummary
	for _, row := range rows {
		switch row.Event {
		case "capture":
			out.CaptureAttempts++
			if row.CandidateCount > 0 {
				out.CandidateCount += row.CandidateCount
			}
			if row.CandidateWrite {
				out.CaptureWritten++
			}
		case "capsule_route":
			if row.Injected {
				out.InjectionCount++
			}
		case "recall":
			out.RecallCount++
			out.RecallHitCount += row.HitCount
			if row.Fallback {
				out.FallbackCount++
			}
			out.Scores = append(out.Scores, row.Scores...)
		}
	}
	return out
}

func summarizeExtract(path string, limit int) (ExtractSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ExtractSummary{ProviderUsage: map[string]int{}}, nil
	}
	recs, err := extract.NewAuditLog(path).Read(limit)
	if err != nil {
		return ExtractSummary{}, err
	}
	var out ExtractSummary
	out.ProviderUsage = map[string]int{}
	for _, rec := range recs {
		out.SessionsScanned += rec.SessionCount
		out.CandidateCount += rec.CandidateCount
		out.Written += rec.Written
		out.Deduped += rec.Deduped
		out.Rejected += rec.Rejected
		out.DriftRefused += rec.DriftRefused
		out.LLMDurationMS += rec.LLMDurationMS
		out.LLMCallCount += rec.LLMCallCount
		if rec.ProviderUsage != "" {
			out.ProviderUsage[rec.ProviderUsage]++
		}
		if rec.LLMUsage != nil {
			if out.LLMUsage == nil {
				out.LLMUsage = &extract.LLMUsage{}
			}
			out.LLMUsage.PromptTokens += rec.LLMUsage.PromptTokens
			out.LLMUsage.CompletionTokens += rec.LLMUsage.CompletionTokens
			out.LLMUsage.TotalTokens += rec.LLMUsage.TotalTokens
		}
		if rec.LLMCost != nil {
			out.LLMCost = mergeExtractCost(out.LLMCost, rec.LLMCost)
		}
	}
	return out, nil
}

func summarizeConsolidate(path string, limit int) (ConsolidateSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ConsolidateSummary{ProviderUsage: map[string]int{}}, nil
	}
	recs, err := consolidate.NewAuditLog(path).Read(limit)
	if err != nil {
		return ConsolidateSummary{}, err
	}
	var out ConsolidateSummary
	out.ProviderUsage = map[string]int{}
	for _, rec := range recs {
		out.CandidateCount += rec.CandidateCount
		out.AddCount += rec.AddCount
		out.UpdateCount += rec.UpdateCount
		out.SkipCount += rec.SkipCount
		if rec.Committed {
			out.Committed++
		}
		if rec.Resynced {
			out.Resynced++
		}
		out.LLMDurationMS += rec.LLMDurationMS
		out.LLMCallCount += rec.LLMCallCount
		if rec.ProviderUsage != "" {
			out.ProviderUsage[rec.ProviderUsage]++
		}
		if rec.LLMUsage != nil {
			if out.LLMUsage == nil {
				out.LLMUsage = &consolidate.LLMUsage{}
			}
			out.LLMUsage.PromptTokens += rec.LLMUsage.PromptTokens
			out.LLMUsage.CompletionTokens += rec.LLMUsage.CompletionTokens
			out.LLMUsage.TotalTokens += rec.LLMUsage.TotalTokens
		}
		if rec.LLMCost != nil {
			out.LLMCost = mergeConsolidateCost(out.LLMCost, rec.LLMCost)
		}
	}
	return out, nil
}

func mergeExtractCost(current, next *extract.LLMCost) *extract.LLMCost {
	if current == nil {
		return next
	}
	amount, ok := addCostAmounts(current.Currency, current.Amount, next.Currency, next.Amount)
	if !ok {
		return current
	}
	return &extract.LLMCost{Currency: current.Currency, Amount: amount}
}

func mergeConsolidateCost(current, next *consolidate.LLMCost) *consolidate.LLMCost {
	if current == nil {
		return next
	}
	amount, ok := addCostAmounts(current.Currency, current.Amount, next.Currency, next.Amount)
	if !ok {
		return current
	}
	return &consolidate.LLMCost{Currency: current.Currency, Amount: amount}
}

func addCostAmounts(currencyA, amountA, currencyB, amountB string) (string, bool) {
	if strings.TrimSpace(currencyA) != strings.TrimSpace(currencyB) {
		return "", false
	}
	a, err := strconv.ParseFloat(strings.TrimSpace(amountA), 64)
	if err != nil {
		return "", false
	}
	b, err := strconv.ParseFloat(strings.TrimSpace(amountB), 64)
	if err != nil {
		return "", false
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(a+b, 'f', 6, 64), "0"), "."), true
}

func reverse[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
