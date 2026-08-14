package sync

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyncStats_RecordSkip(t *testing.T) {
	tests := []struct {
		name  string
		skips int
		want  int
	}{
		{"zero skips", 0, 0},
		{"multiple skips", 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SyncStats
			for i := 0; i < tt.skips; i++ {
				s.RecordSkip()
			}
			assert.Equal(t, tt.want, s.Skipped)
			assert.Equal(t, 0, s.Synced)
		})
	}
}

func TestAnomalyStats(t *testing.T) {
	var a AnomalyStats
	assert.True(t, a.IsZero())
	a.RecordMalformedLines("codex", 2)
	a.RecordMalformedLines("claude", 1)
	a.addSanitize(SanitizeStats{TokensClamped: 3, RoleCoerced: 1})

	assert.False(t, a.IsZero())
	assert.Equal(t, 3, a.MalformedLinesTotal)
	assert.Equal(t, 2, a.MalformedLinesByAgent["codex"])
	assert.Equal(t, 4, a.Sanitize.Total())
}

func TestSyncStatsAnomaliesJSON(t *testing.T) {
	clean, err := json.Marshal(SyncStats{TotalSessions: 1})
	assert.NoError(t, err)
	assert.NotContains(t, string(clean), "anomalies")

	nonzero, err := json.Marshal(SyncStats{Anomalies: AnomalyStats{
		MalformedLinesByAgent: map[string]int{"codex": 2},
		MalformedLinesTotal:   2,
		Sanitize:              SanitizeStats{TokensClamped: 1},
	}})
	assert.NoError(t, err)
	assert.Contains(t, string(nonzero), "anomalies")
	assert.Contains(t, string(nonzero), "malformed_lines_total")
	assert.Contains(t, string(nonzero), "tokens_clamped")
}

func TestAnomalyAccumulatorDedupesMalformedBySource(t *testing.T) {
	var acc anomalyAccumulator
	acc.reset()
	acc.recordMalformedLines("claude", "/tmp/a.jsonl", 3)
	acc.recordMalformedLines("claude", "/tmp/a.jsonl", 3)
	acc.recordMalformedLines("claude", "/tmp/b.jsonl", 2)
	var stats SyncStats
	acc.applyTo(&stats)

	assert.Equal(t, 5, stats.Anomalies.MalformedLinesTotal)
	assert.Equal(t, 5, stats.Anomalies.MalformedLinesByAgent["claude"])

	acc.reset()
	stats = SyncStats{}
	acc.applyTo(&stats)
	assert.True(t, stats.Anomalies.IsZero())
}

func TestSyncStats_RecordSynced(t *testing.T) {
	tests := []struct {
		name   string
		synced []int
		want   int
	}{
		{"zero synced", []int{}, 0},
		{"multiple synced", []int{5, 3}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SyncStats
			for _, v := range tt.synced {
				s.RecordSynced(v)
			}
			assert.Equal(t, 0, s.Skipped)
			assert.Equal(t, tt.want, s.Synced)
		})
	}
}

func TestProgress_Percent(t *testing.T) {
	tests := []struct {
		name string
		p    Progress
		want float64
	}{
		{
			name: "zero total",
			p:    Progress{SessionsTotal: 0, SessionsDone: 0},
			want: 0,
		},
		{
			name: "half done",
			p:    Progress{SessionsTotal: 10, SessionsDone: 5},
			want: 50,
		},
		{
			name: "all done",
			p:    Progress{SessionsTotal: 4, SessionsDone: 4},
			want: 100,
		},
		{
			name: "one third",
			p:    Progress{SessionsTotal: 3, SessionsDone: 1},
			want: 33.333333,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Percent()
			assert.InDelta(t, tt.want, got, 1e-4)
		})
	}
}
