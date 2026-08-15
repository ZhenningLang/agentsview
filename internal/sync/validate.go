package sync

import "go.kenn.io/agentsview/internal/db"

const (
	maxModelLen        = db.MaxModelLen
	maxPlausibleTokens = db.MaxPlausibleTokens
)

type validationStats = db.ValidationStats

func validateAndSanitize(
	s *db.Session, msgs []db.Message, events []db.UsageEvent,
) validationStats {
	var stats validationStats
	if s != nil {
		stats.Add(sanitizeSession(s))
	}
	for i := range msgs {
		stats.Add(sanitizeMessage(&msgs[i]))
	}
	for i := range events {
		stats.Add(sanitizeUsageEvent(&events[i]))
	}
	return stats
}

func sanitizeMessage(m *db.Message) validationStats {
	return db.SanitizeMessage(m)
}

func sanitizeUsageEvent(ev *db.UsageEvent) validationStats {
	return db.SanitizeUsageEvent(ev)
}

func sanitizeSession(s *db.Session) validationStats {
	return db.SanitizeSession(s)
}

func clampedTokens(v int) int {
	return db.ClampPlausibleTokens(v)
}

func blankImplausibleTimestampPtr(p *string) (*string, bool) {
	return db.BlankImplausibleTimestampPtr(p)
}

func usageSourceIsSessionSummary(source string) bool {
	return db.UsageSourceIsSessionSummary(db.SanitizeUTF8(source))
}
