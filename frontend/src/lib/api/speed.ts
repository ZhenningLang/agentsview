import type {
	SpeedBucket,
	SpeedGroupBy,
	SpeedTrendResponse,
} from "./types/analytics.js";
import {
	ApiError,
	authHeaders,
	getBase,
	responseErrorMessage,
} from "./runtime.js";

export async function fetchSpeedTrend(params: {
	since: string;
	until: string;
	bucket: SpeedBucket;
	groupBy: SpeedGroupBy;
	agent?: string;
}): Promise<SpeedTrendResponse> {
	const query = new URLSearchParams({
		since: params.since,
		until: params.until,
		bucket: params.bucket,
		group_by: params.groupBy,
	});
	if (params.agent) query.set("agent", params.agent);
	const response = await fetch(
		`${getBase()}/analytics/speed-trend?${query}`,
		authHeaders(),
	);
	if (!response.ok) {
		throw new ApiError(response.status, await responseErrorMessage(response));
	}
	return (await response.json()) as SpeedTrendResponse;
}
