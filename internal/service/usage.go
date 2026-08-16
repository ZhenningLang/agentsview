package service

import (
	"slices"

	"go.kenn.io/agentsview/internal/db"
)

type PairwiseDimension string

const (
	PairwiseDimensionModel   PairwiseDimension = "model"
	PairwiseDimensionProject PairwiseDimension = "project"
)

type PairwiseSide struct {
	Dimension PairwiseDimension `json:"dimension"`
	Value     string            `json:"value"`
	Empty     bool              `json:"empty,omitempty"`
}

type PairwiseUsageMetrics struct {
	TotalCost           float64  `json:"totalCost"`
	HasCost             bool     `json:"hasCost"`
	UnpricedModels      []string `json:"unpricedModels,omitempty"`
	InputTokens         int      `json:"inputTokens"`
	OutputTokens        int      `json:"outputTokens"`
	CacheCreationTokens int      `json:"cacheCreationTokens"`
	CacheReadTokens     int      `json:"cacheReadTokens"`
	TotalTokens         int      `json:"totalTokens"`
	SessionCount        int      `json:"sessionCount"`
	CostPerSession      *float64 `json:"costPerSession,omitempty"`
	TokensPerSession    *float64 `json:"tokensPerSession,omitempty"`
}

type PairwiseDelta struct {
	TotalCost            float64  `json:"totalCost"`
	HasCost              bool     `json:"hasCost"`
	UnpricedModels       []string `json:"unpricedModels,omitempty"`
	InputTokens          int      `json:"inputTokens"`
	OutputTokens         int      `json:"outputTokens"`
	CacheCreationTokens  int      `json:"cacheCreationTokens"`
	CacheReadTokens      int      `json:"cacheReadTokens"`
	TotalTokens          int      `json:"totalTokens"`
	SessionCount         int      `json:"sessionCount"`
	CostPerSession       *float64 `json:"costPerSession,omitempty"`
	TokensPerSession     *float64 `json:"tokensPerSession,omitempty"`
	CostRelativeChange   *float64 `json:"costRelativeChange,omitempty"`
	TokensRelativeChange *float64 `json:"tokensRelativeChange,omitempty"`
}

type PairwiseComparisonResponse struct {
	Left         PairwiseSide         `json:"left"`
	Right        PairwiseSide         `json:"right"`
	LeftMetrics  PairwiseUsageMetrics `json:"leftMetrics"`
	RightMetrics PairwiseUsageMetrics `json:"rightMetrics"`
	Deltas       PairwiseDelta        `json:"deltas"`
}

func UsageMetricsFromResult(result db.DailyUsageResult) PairwiseUsageMetrics {
	totals := result.Totals
	sessionCount := result.SessionCounts.Total
	totalTokens := totals.InputTokens + totals.OutputTokens +
		totals.CacheCreationTokens + totals.CacheReadTokens
	out := PairwiseUsageMetrics{
		TotalCost:           totals.TotalCost,
		HasCost:             totals.HasCost,
		UnpricedModels:      totals.UnpricedModels,
		InputTokens:         totals.InputTokens,
		OutputTokens:        totals.OutputTokens,
		CacheCreationTokens: totals.CacheCreationTokens,
		CacheReadTokens:     totals.CacheReadTokens,
		TotalTokens:         totalTokens,
		SessionCount:        sessionCount,
	}
	if sessionCount > 0 {
		if totals.HasCost {
			cost := totals.TotalCost / float64(sessionCount)
			out.CostPerSession = &cost
		}
		tokens := float64(totalTokens) / float64(sessionCount)
		out.TokensPerSession = &tokens
	}
	return out
}

func PairwiseDeltas(left, right PairwiseUsageMetrics) PairwiseDelta {
	out := PairwiseDelta{
		TotalCost:           right.TotalCost - left.TotalCost,
		HasCost:             left.HasCost && right.HasCost,
		InputTokens:         right.InputTokens - left.InputTokens,
		OutputTokens:        right.OutputTokens - left.OutputTokens,
		CacheCreationTokens: right.CacheCreationTokens - left.CacheCreationTokens,
		CacheReadTokens:     right.CacheReadTokens - left.CacheReadTokens,
		TotalTokens:         right.TotalTokens - left.TotalTokens,
		SessionCount:        right.SessionCount - left.SessionCount,
	}
	if !out.HasCost {
		out.UnpricedModels = sortedUniqueStrings(append(
			append([]string{}, left.UnpricedModels...), right.UnpricedModels...))
	}
	if left.CostPerSession != nil && right.CostPerSession != nil {
		v := *right.CostPerSession - *left.CostPerSession
		out.CostPerSession = &v
	}
	if left.TokensPerSession != nil && right.TokensPerSession != nil {
		v := *right.TokensPerSession - *left.TokensPerSession
		out.TokensPerSession = &v
	}
	if out.HasCost && left.TotalCost != 0 {
		v := out.TotalCost / left.TotalCost
		out.CostRelativeChange = &v
	}
	if left.TotalTokens != 0 {
		v := float64(out.TotalTokens) / float64(left.TotalTokens)
		out.TokensRelativeChange = &v
	}
	return out
}

func BuildPairwiseComparison(
	left PairwiseSide, leftResult db.DailyUsageResult,
	right PairwiseSide, rightResult db.DailyUsageResult,
) PairwiseComparisonResponse {
	leftMetrics := UsageMetricsFromResult(leftResult)
	rightMetrics := UsageMetricsFromResult(rightResult)
	return PairwiseComparisonResponse{
		Left:         left,
		Right:        right,
		LeftMetrics:  leftMetrics,
		RightMetrics: rightMetrics,
		Deltas:       PairwiseDeltas(leftMetrics, rightMetrics),
	}
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if v != "" {
			seen[v] = true
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}
