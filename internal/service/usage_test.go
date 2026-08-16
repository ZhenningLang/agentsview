package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

func TestPhase17PairwiseDeltasCarryIncompleteCost(t *testing.T) {
	left := service.PairwiseUsageMetrics{
		TotalCost: 1, HasCost: true, TotalTokens: 100,
	}
	right := service.PairwiseUsageMetrics{
		TotalCost: 0.5, HasCost: false, TotalTokens: 50,
		UnpricedModels: []string{"local-model"},
	}

	got := service.PairwiseDeltas(left, right)

	assert.False(t, got.HasCost)
	assert.Equal(t, []string{"local-model"}, got.UnpricedModels)
	assert.Nil(t, got.CostRelativeChange)
	assert.NotNil(t, got.TokensRelativeChange)
}

func TestPhase17BuildPairwiseComparisonCarriesEmptySide(t *testing.T) {
	got := service.BuildPairwiseComparison(
		service.PairwiseSide{Dimension: service.PairwiseDimensionProject, Value: "missing", Empty: true},
		db.DailyUsageResult{},
		service.PairwiseSide{Dimension: service.PairwiseDimensionProject, Value: "alpha"},
		db.DailyUsageResult{Totals: db.UsageTotals{HasCost: true}, SessionCounts: db.UsageSessionCounts{Total: 1}},
	)

	assert.True(t, got.Left.Empty)
	assert.False(t, got.Right.Empty)
	assert.Zero(t, got.LeftMetrics.TotalTokens)
}
