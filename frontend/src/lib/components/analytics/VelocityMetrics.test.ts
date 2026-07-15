// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";
import type { VelocityResponse } from "../../api/types/analytics.js";

const state = vi.hoisted(() => ({
	analytics: {
		velocity: {
			overall: {
				turn_cycle_sec: { p50: 1, p90: 2 },
				first_response_sec: { p50: 1, p90: 2 },
				msgs_per_active_min: 1,
				chars_per_active_min: 1,
				tool_calls_per_active_min: 0,
				output_tok_per_sec_p50: null,
				output_tok_per_sec_p95: null,
				speed_n: 3,
			},
			by_agent: [] as VelocityResponse["by_agent"],
			by_complexity: [] as VelocityResponse["by_complexity"],
		} as VelocityResponse,
		errors: { velocity: null },
		fetchVelocity: vi.fn(),
	},
}));

vi.mock("../../stores/analytics.svelte.js", () => state);
// @ts-ignore Svelte components are compiled by the test runner.
import VelocityMetrics from "./VelocityMetrics.svelte";

describe("VelocityMetrics", () => {
	let component: ReturnType<typeof mount> | undefined;

	afterEach(() => {
		if (component) unmount(component);
		component = undefined;
		document.body.innerHTML = "";
	});

	it("renders insufficient-data speed cards without hiding other velocity metrics", async () => {
		component = mount(VelocityMetrics, { target: document.body });
		await tick();
		expect(document.body.textContent).toContain("Output speed p50 (approx.)");
		expect(document.body.textContent).toContain("insufficient data");
		expect(document.body.textContent).toContain("Turn Cycle (p50)");
	});

	it("labels breakdown speed as an approximate p50", async () => {
		state.analytics.velocity.by_agent = [{
			label: "claude",
			sessions: 1,
			overview: state.analytics.velocity.overall,
		}];
		component = mount(VelocityMetrics, { target: document.body });
		await tick();
		(document.querySelectorAll("button")[1] as HTMLButtonElement).click();
		await tick();
		expect(document.body.textContent).toContain("tok/s p50 (approx.)");
	});
});
