package service

import (
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/timeutil"
)

// This file owns the single implementation of the usage read model: the
// query defaults, the filter that those defaults produce, the folds from
// daily rows to range-wide totals, and the pairwise dimension
// intersection. The HTTP routes and the direct/HTTP service backends all
// go through it. A second copy anywhere would drift, and a drifted fold
// or default does not fail a test: it just makes two entry points report
// different numbers for the same range.

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

// UsageSummaryResult is the transport-neutral usage summary. It is also
// the JSON shape of GET /api/v1/usage/summary: the route aliases this
// type rather than declaring a parallel one.
type UsageSummaryResult struct {
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

// UsageInputError marks a caller-supplied usage parameter as invalid.
// The HTTP routes map it to 400; the direct backend returns it as-is so
// a CLI or tool caller gets the same message without a server.
type UsageInputError struct{ Msg string }

func (e *UsageInputError) Error() string { return e.Msg }

// UsageRequest is the transport-neutral usage input. Each field maps to
// one query parameter of the usage routes.
//
// IncludeOneShot and IncludeAutomated are pointers because a plain bool
// cannot express both wire defaults at once: include_one_shot defaults
// to true and include_automated to false. nil means "use the documented
// default", which is what an omitted query parameter does, so a direct
// caller and an HTTP caller that both leave them unset get the same
// rows. Flipping one of these silently changes every number the endpoint
// reports without failing anything.
type UsageRequest struct {
	From             string
	To               string
	Timezone         string
	Agent            string
	Project          string
	Machine          string
	GitBranch        string
	ExcludeProject   string
	ExcludeAgent     string
	ExcludeModel     string
	Model            string
	MinUserMessages  int
	ActiveSince      string
	IncludeOneShot   *bool
	IncludeAutomated *bool

	// NoCache only reaches a server: the cache lives in the HTTP layer,
	// so the direct backend always computes fresh and ignores this.
	NoCache bool
}

// UsagePairwiseComparisonRequest compares two dimension values over the
// same range. The embedded UsageRequest is the shared filter; each side
// then intersects it with its own dimension value.
type UsagePairwiseComparisonRequest struct {
	UsageRequest
	LeftDimension  PairwiseDimension
	LeftValue      string
	RightDimension PairwiseDimension
	RightValue     string
}

// DefaultDateRange returns (from, to) defaulting to the last 30 days
// ending today (UTC). An unparseable to date falls back to today so the
// caller still gets a well-formed range to validate.
func DefaultDateRange(from, to string) (string, string) {
	now := time.Now().UTC()
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if from == "" {
		t, err := time.Parse("2006-01-02", to)
		if err != nil {
			t = now
		}
		from = t.AddDate(0, 0, -30).Format("2006-01-02")
	}
	return from, to
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// UsageFilterFromRequest applies the defaults and validation shared by
// every usage entry point and returns the store filter they all run.
// Breakdowns is on; the pairwise path turns it off explicitly, matching
// what the route does.
func UsageFilterFromRequest(req UsageRequest) (db.UsageFilter, error) {
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return db.UsageFilter{}, &UsageInputError{Msg: "invalid timezone: " + tz}
	}
	from, to := DefaultDateRange(req.From, req.To)
	if !timeutil.IsValidDate(from) || !timeutil.IsValidDate(to) {
		return db.UsageFilter{}, &UsageInputError{
			Msg: "invalid date format: use YYYY-MM-DD"}
	}
	if from > to {
		return db.UsageFilter{}, &UsageInputError{
			Msg: "from must not be after to"}
	}
	if req.ActiveSince != "" && !timeutil.IsValidTimestamp(req.ActiveSince) {
		return db.UsageFilter{}, &UsageInputError{
			Msg: "invalid active_since: use RFC3339 timestamp"}
	}
	return db.UsageFilter{
		From:             from,
		To:               to,
		Agent:            req.Agent,
		Project:          req.Project,
		Machine:          req.Machine,
		GitBranch:        req.GitBranch,
		ExcludeProject:   req.ExcludeProject,
		ExcludeAgent:     req.ExcludeAgent,
		ExcludeModel:     req.ExcludeModel,
		Model:            req.Model,
		Timezone:         tz,
		MinUserMessages:  req.MinUserMessages,
		ExcludeOneShot:   !boolOr(req.IncludeOneShot, true),
		ExcludeAutomated: !boolOr(req.IncludeAutomated, false),
		ActiveSince:      req.ActiveSince,
		Breakdowns:       true,
	}, nil
}

// BuildUsageSummary folds a store result into the summary shape. The
// range comes from the filter that produced the result, so the echoed
// from/to are always the defaulted values rather than the raw input.
func BuildUsageSummary(
	f db.UsageFilter, result db.DailyUsageResult,
) *UsageSummaryResult {
	return &UsageSummaryResult{
		From:          f.From,
		To:            f.To,
		Totals:        result.Totals,
		Daily:         result.Daily,
		ProjectTotals: foldProjectTotals(result.Daily),
		ModelTotals:   foldModelTotals(result.Daily),
		AgentTotals:   foldAgentTotals(result.Daily),
		MachineTotals: foldMachineTotals(result.Daily),
		SessionCounts: result.SessionCounts,
		CacheStats:    ComputeCacheStats(result.Totals),
	}
}

// ParsePairwiseDimension validates a dimension name.
func ParsePairwiseDimension(raw string) (PairwiseDimension, error) {
	switch PairwiseDimension(raw) {
	case PairwiseDimensionModel:
		return PairwiseDimensionModel, nil
	case PairwiseDimensionProject:
		return PairwiseDimensionProject, nil
	default:
		return "", &UsageInputError{Msg: "invalid pairwise dimension: " + raw}
	}
}

// PairwiseFilter narrows f to one side of a comparison. The bool reports
// an empty intersection: the requested value is excluded by the shared
// filter, so that side has no rows at all and must not be queried.
func PairwiseFilter(
	f db.UsageFilter, dim PairwiseDimension, value string,
) (db.UsageFilter, bool) {
	f.Breakdowns = false
	var empty bool
	switch dim {
	case PairwiseDimensionModel:
		f.Model, empty = IntersectCSV(f.Model, value)
	case PairwiseDimensionProject:
		f.Project, empty = IntersectCSV(f.Project, value)
	}
	return f, empty
}

// IntersectCSV intersects a comma-separated filter with a single value.
// An empty base means "no constraint", so the value wins.
func IntersectCSV(base, value string) (string, bool) {
	if base == "" {
		return value, false
	}
	for part := range strings.SplitSeq(base, ",") {
		if strings.TrimSpace(part) == value {
			return value, false
		}
	}
	return "", true
}

// EmptyUsageResult is the zero result for a side with an empty
// intersection: no daily rows and initialised count maps, so callers
// fold it exactly like a queried result.
func EmptyUsageResult() db.DailyUsageResult {
	return db.DailyUsageResult{
		Daily: []db.DailyUsageEntry{},
		SessionCounts: db.UsageSessionCounts{
			ByProject: map[string]int{},
			ByAgent:   map[string]int{},
		},
	}
}

// ComputeCacheStats derives cache hit/miss metrics from totals.
// SavingsVsUncached passes through totals.CacheSavings, which the DB
// layer computes per-message using each row's actual per-model rates -
// so mixed-model periods (e.g. Opus + Sonnet) report the right net delta
// instead of a single hard-coded proxy rate.
func ComputeCacheStats(t db.UsageTotals) CacheStats {
	// Anthropic reports input_tokens as the NON-cached portion of the
	// input (cache_read and cache_creation are separate fields), so
	// UncachedInputTokens is just t.InputTokens directly - no
	// subtraction.
	cs := CacheStats{
		CacheReadTokens:     t.CacheReadTokens,
		CacheCreationTokens: t.CacheCreationTokens,
		UncachedInputTokens: t.InputTokens,
		OutputTokens:        t.OutputTokens,
		SavingsVsUncached:   t.CacheSavings,
	}
	denominator := t.CacheReadTokens + t.InputTokens
	if denominator > 0 {
		cs.HitRate = float64(t.CacheReadTokens) / float64(denominator)
	}
	return cs
}

// foldProjectTotals sums daily project breakdowns into range-wide totals
// sorted by cost descending.
func foldProjectTotals(daily []db.DailyUsageEntry) []ProjectTotal {
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

// foldModelTotals sums daily model breakdowns into range-wide totals
// sorted by cost descending.
func foldModelTotals(daily []db.DailyUsageEntry) []ModelTotal {
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

// foldAgentTotals sums daily agent breakdowns into range-wide totals
// sorted by cost descending.
func foldAgentTotals(daily []db.DailyUsageEntry) []AgentTotal {
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

// foldMachineTotals sums daily machine breakdowns into range-wide totals
// sorted by cost descending.
func foldMachineTotals(daily []db.DailyUsageEntry) []MachineTotal {
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
