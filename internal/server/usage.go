package server

import (
	"sort"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

// ProjectTotal holds range-wide token and cost totals per project.
type ProjectTotal struct {
	Project             string  `json:"project"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	Cost                float64 `json:"cost"`
}

// ModelTotal holds range-wide token and cost totals per model.
type ModelTotal struct {
	Model               string  `json:"model"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	Cost                float64 `json:"cost"`
}

// AgentTotal holds range-wide token and cost totals per agent.
type AgentTotal struct {
	Agent               string  `json:"agent"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	Cost                float64 `json:"cost"`
}

// MachineTotal holds range-wide token and cost totals per machine.
type MachineTotal struct {
	Machine             string  `json:"machine"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens"`
	Cost                float64 `json:"cost"`
}

// CacheStats summarizes cache hit/miss for the period.
type CacheStats struct {
	CacheReadTokens     int     `json:"cacheReadTokens"`
	CacheCreationTokens int     `json:"cacheCreationTokens"`
	UncachedInputTokens int     `json:"uncachedInputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	HitRate             float64 `json:"hitRate"`
	SavingsVsUncached   float64 `json:"savingsVsUncached"`
}

// Comparison holds the prior-period cost comparison.
type Comparison struct {
	PriorFrom      string  `json:"priorFrom"`
	PriorTo        string  `json:"priorTo"`
	PriorTotalCost float64 `json:"priorTotalCost"`
	DeltaPct       float64 `json:"deltaPct"`
}

// UsageSummaryResponse is the JSON shape for
// GET /api/v1/usage/summary.
type UsageSummaryResponse struct {
	From          string                `json:"from"`
	To            string                `json:"to"`
	Totals        db.UsageTotals        `json:"totals"`
	Daily         []db.DailyUsageEntry  `json:"daily"`
	ProjectTotals []ProjectTotal        `json:"projectTotals"`
	ModelTotals   []ModelTotal          `json:"modelTotals"`
	AgentTotals   []AgentTotal          `json:"agentTotals"`
	MachineTotals []MachineTotal        `json:"machineTotals"`
	SessionCounts db.UsageSessionCounts `json:"sessionCounts"`
	CacheStats    CacheStats            `json:"cacheStats"`
	Comparison    *Comparison           `json:"comparison,omitempty"`
}

type PairwiseDimension = service.PairwiseDimension
type PairwiseSide = service.PairwiseSide
type PairwiseUsageMetrics = service.PairwiseUsageMetrics
type PairwiseDelta = service.PairwiseDelta
type PairwiseComparisonResponse = service.PairwiseComparisonResponse

const (
	PairwiseDimensionModel   = service.PairwiseDimensionModel
	PairwiseDimensionProject = service.PairwiseDimensionProject
)

// foldProjectTotals sums daily project breakdowns into
// range-wide totals sorted by cost descending.
func foldProjectTotals(
	daily []db.DailyUsageEntry,
) []ProjectTotal {
	m := make(map[string]*ProjectTotal)
	for _, d := range daily {
		if len(d.ProjectBreakdowns) == 0 {
			continue
		}
		for _, pb := range d.ProjectBreakdowns {
			pt, ok := m[pb.Project]
			if !ok {
				pt = &ProjectTotal{Project: pb.Project}
				m[pb.Project] = pt
			}
			pt.InputTokens += pb.InputTokens
			pt.OutputTokens += pb.OutputTokens
			pt.CacheCreationTokens += pb.CacheCreationTokens
			pt.CacheReadTokens += pb.CacheReadTokens
			pt.Cost += pb.Cost
		}
	}
	out := make([]ProjectTotal, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Project < out[j].Project
	})
	return out
}

// foldModelTotals sums daily model breakdowns into range-wide
// totals sorted by cost descending.
func foldModelTotals(
	daily []db.DailyUsageEntry,
) []ModelTotal {
	m := make(map[string]*ModelTotal)
	for _, d := range daily {
		if len(d.ModelBreakdowns) == 0 {
			continue
		}
		for _, mb := range d.ModelBreakdowns {
			mt, ok := m[mb.ModelName]
			if !ok {
				mt = &ModelTotal{Model: mb.ModelName}
				m[mb.ModelName] = mt
			}
			mt.InputTokens += mb.InputTokens
			mt.OutputTokens += mb.OutputTokens
			mt.CacheCreationTokens += mb.CacheCreationTokens
			mt.CacheReadTokens += mb.CacheReadTokens
			mt.Cost += mb.Cost
		}
	}
	out := make([]ModelTotal, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// foldAgentTotals sums daily agent breakdowns into range-wide
// totals sorted by cost descending.
func foldAgentTotals(
	daily []db.DailyUsageEntry,
) []AgentTotal {
	m := make(map[string]*AgentTotal)
	for _, d := range daily {
		if len(d.AgentBreakdowns) == 0 {
			continue
		}
		for _, ab := range d.AgentBreakdowns {
			at, ok := m[ab.Agent]
			if !ok {
				at = &AgentTotal{Agent: ab.Agent}
				m[ab.Agent] = at
			}
			at.InputTokens += ab.InputTokens
			at.OutputTokens += ab.OutputTokens
			at.CacheCreationTokens += ab.CacheCreationTokens
			at.CacheReadTokens += ab.CacheReadTokens
			at.Cost += ab.Cost
		}
	}
	out := make([]AgentTotal, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Agent < out[j].Agent
	})
	return out
}

func foldMachineTotals(
	daily []db.DailyUsageEntry,
) []MachineTotal {
	m := make(map[string]*MachineTotal)
	for _, d := range daily {
		if len(d.MachineBreakdowns) == 0 {
			continue
		}
		for _, mb := range d.MachineBreakdowns {
			mt, ok := m[mb.Machine]
			if !ok {
				mt = &MachineTotal{Machine: mb.Machine}
				m[mb.Machine] = mt
			}
			mt.InputTokens += mb.InputTokens
			mt.OutputTokens += mb.OutputTokens
			mt.CacheCreationTokens += mb.CacheCreationTokens
			mt.CacheReadTokens += mb.CacheReadTokens
			mt.Cost += mb.Cost
		}
	}
	out := make([]MachineTotal, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Machine < out[j].Machine
	})
	return out
}

// computeCacheStats derives cache hit/miss metrics from totals.
// SavingsVsUncached passes through totals.CacheSavings, which
// the DB layer computes per-message using each row's actual
// per-model rates — so mixed-model periods (e.g. Opus + Sonnet)
// report the right net delta instead of a single hard-coded
// proxy rate.
func computeCacheStats(t db.UsageTotals) CacheStats {
	// Anthropic reports input_tokens as the NON-cached portion
	// of the input (cache_read and cache_creation are separate
	// fields), so UncachedInputTokens is just t.InputTokens
	// directly — no subtraction.
	cs := CacheStats{
		CacheReadTokens:     t.CacheReadTokens,
		CacheCreationTokens: t.CacheCreationTokens,
		UncachedInputTokens: t.InputTokens,
		OutputTokens:        t.OutputTokens,
		SavingsVsUncached:   t.CacheSavings,
	}
	denominator := t.CacheReadTokens + t.InputTokens
	if denominator > 0 {
		cs.HitRate = float64(t.CacheReadTokens) /
			float64(denominator)
	}
	return cs
}
