package db

import "github.com/tidwall/gjson"

// UsageTokenValues normalizes one usage row for analytics. Message rows read
// token counts from raw token_usage JSON; non-summary rows are clamped to the
// row-level plausibility bound, while session summaries only floor negatives.
func UsageTokenValues(
	source, tokenJSON string,
	inputTokens, outputTokens,
	cacheCreationTokens, cacheReadTokens int,
) (input, output, cacheCreation, cacheRead int) {
	if source == "message" {
		usage := gjson.Parse(tokenJSON)
		inputTokens = int(usage.Get("input_tokens").Int())
		outputTokens = int(usage.Get("output_tokens").Int())
		cacheCreationTokens = int(
			usage.Get("cache_creation_input_tokens").Int(),
		)
		cacheReadTokens = int(
			usage.Get("cache_read_input_tokens").Int(),
		)
	}
	if UsageSourceIsSessionSummary(source) {
		return floorNegative(inputTokens),
			floorNegative(outputTokens),
			floorNegative(cacheCreationTokens),
			floorNegative(cacheReadTokens)
	}
	return ClampPlausibleTokens(inputTokens),
		ClampPlausibleTokens(outputTokens),
		ClampPlausibleTokens(cacheCreationTokens),
		ClampPlausibleTokens(cacheReadTokens)
}

func floorNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
