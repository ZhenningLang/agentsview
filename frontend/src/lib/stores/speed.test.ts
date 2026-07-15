import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchSpeedTrend = vi.fn();

vi.mock("../api/speed.js", () => ({ fetchSpeedTrend }));

describe("SpeedStore", () => {
	beforeEach(() => {
		vi.resetModules();
		fetchSpeedTrend.mockReset();
	});

	it("requests the selected group, bucket, and time range", async () => {
		fetchSpeedTrend.mockResolvedValue({
			bucket_sec: 900,
			group_by: "model",
			since: "2026-07-14T00:00:00Z",
			until: "2026-07-15T00:00:00Z",
			series: [],
		});
		const { speed } = await import("./speed.svelte.js");
		speed.bucket = "15m";
		speed.groupBy = "model";
		speed.rangeHours = 24;
		await speed.fetch();
		expect(fetchSpeedTrend).toHaveBeenCalledWith(expect.objectContaining({
			bucket: "15m",
			groupBy: "model",
		}));
		expect(speed.response?.group_by).toBe("model");
	});

	it("retains a load error", async () => {
		fetchSpeedTrend.mockRejectedValue(new Error("network down"));
		const { speed } = await import("./speed.svelte.js");
		await speed.fetch();
		expect(speed.error).toBe("network down");
	});
});
