package postgres

import (
	"context"
	"fmt"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Store) GetEnrichmentStatus(ctx context.Context) (db.EnrichmentStatusReport, error) {
	rows, err := s.pg.QueryContext(ctx, `
		SELECT COALESCE(enrich_status, ''), COUNT(*)
		FROM sessions
		WHERE deleted_at IS NULL
		GROUP BY COALESCE(enrich_status, '')`)
	if err != nil {
		return db.EnrichmentStatusReport{}, fmt.Errorf("query enrichment status: %w", err)
	}
	defer rows.Close()
	report := db.EnrichmentStatusReport{ByStatus: map[string]int{}}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return db.EnrichmentStatusReport{}, fmt.Errorf("scan enrichment status: %w", err)
		}
		db.AccumulateEnrichmentStatus(&report, status, count)
	}
	if err := rows.Err(); err != nil {
		return db.EnrichmentStatusReport{}, fmt.Errorf("iterate enrichment status: %w", err)
	}
	return report, nil
}
