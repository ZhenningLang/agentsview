package server

import (
	"context"
	"fmt"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Server) enrichSessionTimingSpeed(ctx context.Context, timing *db.SessionTiming) error {
	result, err := s.db.GetSessionSpeed(ctx, timing.SessionID)
	if err != nil {
		return err
	}
	if result.Speed == nil {
		timing.Speed = nil
		return nil
	}
	baselineP50, baselineN, err := s.cachedSpeedBaseline(ctx, result.Agent, timing.SessionID)
	if err != nil {
		return err
	}
	result.Speed.BaselineP50 = baselineP50
	result.Speed.BaselineN = baselineN
	timing.Speed = result.Speed
	return nil
}

func (s *Server) cachedSpeedBaseline(ctx context.Context, agent, sessionID string) (*float64, int, error) {
	if s.speedBaselineCache == nil {
		rates, err := s.db.GetSpeedBaselineSessions(ctx, agent, time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC())
		if err != nil {
			return nil, 0, err
		}
		p50, n := db.SessionSpeedBaselineExcluding(rates, sessionID)
		return p50, n, nil
	}
	key := fmt.Sprintf("%T|%s", s.db, agent)
	now := time.Now().UTC()
	if rates, ok := s.speedBaselineCache.get(key, now); ok {
		p50, n := db.SessionSpeedBaselineExcluding(rates, sessionID)
		return p50, n, nil
	}
	rates, err := s.db.GetSpeedBaselineSessions(ctx, agent, now.AddDate(0, 0, -30), now)
	if err != nil {
		return nil, 0, err
	}
	s.speedBaselineCache.set(key, rates, now)
	p50, n := db.SessionSpeedBaselineExcluding(rates, sessionID)
	return p50, n, nil
}
