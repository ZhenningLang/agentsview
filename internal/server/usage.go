package server

import "go.kenn.io/agentsview/internal/service"

// The usage read model - request defaults, the store filter they build,
// the folds to range-wide totals, and the cache-stat derivation - lives
// in internal/service so the HTTP routes and the service backends share
// one implementation. The types below are aliases, not copies: a second
// fold or default here would drift and make the two entry points report
// different numbers for the same range without failing a test.

type ProjectTotal = service.ProjectTotal
type ModelTotal = service.ModelTotal
type AgentTotal = service.AgentTotal
type MachineTotal = service.MachineTotal
type CacheStats = service.CacheStats
type Comparison = service.Comparison

// UsageSummaryResponse is the JSON shape for GET /api/v1/usage/summary.
type UsageSummaryResponse = service.UsageSummaryResult

type PairwiseDimension = service.PairwiseDimension
type PairwiseSide = service.PairwiseSide
type PairwiseUsageMetrics = service.PairwiseUsageMetrics
type PairwiseDelta = service.PairwiseDelta
type PairwiseComparisonResponse = service.PairwiseComparisonResponse

const (
	PairwiseDimensionModel   = service.PairwiseDimensionModel
	PairwiseDimensionProject = service.PairwiseDimensionProject
)
