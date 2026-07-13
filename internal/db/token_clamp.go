package db

// MaxPlausibleTokens bounds one parsed row-level token field. Session totals
// may legitimately exceed this value by summing multiple rows.
const MaxPlausibleTokens = 2_000_000

// ClampPlausibleTokens bounds one parsed token field to the accepted range.
func ClampPlausibleTokens(v int) int {
	switch {
	case v < 0:
		return 0
	case v > MaxPlausibleTokens:
		return MaxPlausibleTokens
	default:
		return v
	}
}
