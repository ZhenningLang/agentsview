package server

import "go.kenn.io/agentsview/internal/service"

// defaultDateRange returns (from, to) defaulting to the last 30 days if
// not provided. It delegates so the analytics routes and the usage read
// model cannot end up with two different notions of "no range given".
func defaultDateRange(from, to string) (string, string) {
	return service.DefaultDateRange(from, to)
}
