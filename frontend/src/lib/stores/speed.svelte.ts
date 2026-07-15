import { fetchSpeedTrend } from "../api/speed.js";
import type {
	SpeedBucket,
	SpeedGroupBy,
	SpeedTrendResponse,
} from "../api/types/analytics.js";

function isoHoursAgo(hours: number): string {
	return new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();
}

class SpeedStore {
	bucket: SpeedBucket = $state("hour");
	groupBy: SpeedGroupBy = $state("agent");
	rangeHours = $state(168);
	response: SpeedTrendResponse | null = $state(null);
	loading = $state(false);
	error: string | null = $state(null);
	private version = 0;

	async fetch(): Promise<void> {
		const version = ++this.version;
		this.loading = true;
		this.error = null;
		try {
			const until = new Date().toISOString();
			const response = await fetchSpeedTrend({
				since: isoHoursAgo(this.rangeHours),
				until,
				bucket: this.bucket,
				groupBy: this.groupBy,
			});
			if (version === this.version) this.response = response;
		} catch (error) {
			if (version === this.version) {
				this.error = error instanceof Error ? error.message : "Failed to load speed trend";
			}
		} finally {
			if (version === this.version) this.loading = false;
		}
	}
}

export const speed = new SpeedStore();
