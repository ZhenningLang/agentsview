package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifierHashStable(t *testing.T) {
	t.Cleanup(func() { SetUserAutomationPrefixes(nil) })
	SetUserAutomationPrefixes([]string{"foo", "bar"})
	a := ClassifierHash()
	b := ClassifierHash()
	assert.Equal(t, a, b, "hash unstable")
}

func TestClassifierHashChangesWithUserPrefixes(t *testing.T) {
	t.Cleanup(func() { SetUserAutomationPrefixes(nil) })
	SetUserAutomationPrefixes(nil)
	base := ClassifierHash()
	SetUserAutomationPrefixes([]string{"You are analyzing an essay"})
	with := ClassifierHash()
	assert.NotEqual(t, base, with,
		"hash did not change when user prefixes changed")
}

func TestPhase16ClassifierHashIncludesMatcherKinds(t *testing.T) {
	t.Cleanup(func() { setPhase16AutomationConfigForTest(nil, nil, nil) })
	setPhase16AutomationConfigForTest(nil, nil, nil)
	base := ClassifierHash()

	setPhase16AutomationConfigForTest(nil, []string{"phase16 hash marker"}, nil)
	withSubstring := ClassifierHash()
	assert.NotEqual(t, base, withSubstring,
		"hash did not change when user substrings changed")

	setPhase16AutomationConfigForTest(nil, nil, []string{"phase16 exact marker"})
	withExact := ClassifierHash()
	assert.NotEqual(t, base, withExact,
		"hash did not change when user exact matches changed")

	setPhase16AutomationConfigForTest([]string{"phase16 same text"}, nil, nil)
	prefixHash := ClassifierHash()
	setPhase16AutomationConfigForTest(nil, []string{"phase16 same text"}, nil)
	substringHash := ClassifierHash()
	setPhase16AutomationConfigForTest(nil, nil, []string{"phase16 same text"})
	exactHash := ClassifierHash()
	assert.NotEqual(t, prefixHash, substringHash,
		"same text as prefix and substring should hash differently")
	assert.NotEqual(t, substringHash, exactHash,
		"same text as substring and exact match should hash differently")

	setPhase16AutomationConfigForTest(nil,
		[]string{"beta marker", "alpha marker", "alpha marker"},
		[]string{"omega marker"})
	a := ClassifierHash()
	setPhase16AutomationConfigForTest(nil,
		[]string{"alpha marker", "beta marker"},
		[]string{"omega marker", "omega marker"})
	b := ClassifierHash()
	assert.Equal(t, a, b,
		"hash should be deterministic for duplicate/order-equivalent config")
}

func setPhase16AutomationConfigForTest(prefixes, substrings, exact []string) {
	SetUserAutomationMatchers(prefixes, substrings, exact)
}

func TestClassifierHashOrderIndependent(t *testing.T) {
	t.Cleanup(func() { SetUserAutomationPrefixes(nil) })
	SetUserAutomationPrefixes([]string{"alpha", "beta", "gamma"})
	a := ClassifierHash()
	SetUserAutomationPrefixes([]string{"gamma", "alpha", "beta"})
	b := ClassifierHash()
	assert.Equal(t, a, b, "hash not order-independent")
}

// TestClassifierHashTagSeparation guards against the case
// where two different categorizations produce the same hash
// because the tag prefix was dropped from the encoding.
func TestClassifierHashTagSeparation(t *testing.T) {
	t.Cleanup(func() { SetUserAutomationPrefixes(nil) })
	SetUserAutomationPrefixes([]string{"Warmup"})
	got := ClassifierHash()
	SetUserAutomationPrefixes(nil)
	bareBuiltins := ClassifierHash()
	assert.NotEqual(t, got, bareBuiltins,
		"user prefix 'Warmup' collided with built-in exact-match 'Warmup'")
}

// TestClassifierHashCurrentAlgoVersion is a forced-bump
// guard: it pins the algorithm version at construction time.
// If a future change to the matching logic forgets to bump
// classifierAlgorithmVersion, this test still passes (false
// negative) — but if someone bumps the version intentionally
// the test must be updated to match. The check exists to
// surface accidental version-constant edits during review.
func TestClassifierHashCurrentAlgoVersion(t *testing.T) {
	assert.Equal(t, 2, classifierAlgorithmVersion,
		"classifierAlgorithmVersion changed; update this test and confirm "+
			"matching semantics actually changed (not just pattern edits, "+
			"which the hash already detects)")
}
