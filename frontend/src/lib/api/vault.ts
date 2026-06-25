import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// VaultPhase mirrors db.VaultPhase: one phase directory of a dev-long-run.
// Pointer fields on the backend become optional fields here; absent means the
// corresponding optional file (verify.json / stuck.json) was not present.
export interface VaultPhase {
  run_slug: string;
  phase_id: string;
  verify_ok?: boolean;
  verify_exit?: number;
  stuck_consecutive_fail?: number;
  stuck_fingerprint?: string;
}

// VaultMetric mirrors db.VaultMetric: one event row from metrics.jsonl.
// dev-complete runs carry no metrics. ok/exit/fingerprint are absent for
// events that do not carry them (e.g. complete_phase).
export interface VaultMetric {
  run_slug: string;
  ts: string;
  event: string;
  phase?: string;
  ok?: boolean;
  exit?: number;
  fingerprint?: string;
}

// VaultRun mirrors db.VaultRun: one dev-workflow run discovered under
// `.long-loop/<slug>/`. A run is either a dev-long-run (multi-phase, phases +
// metrics populated) or a dev-complete (single pass, no phases/metrics). The
// `skill` field distinguishes them. acceptance_ok / acceptance_exit are absent
// when no acceptance.json was present (incomplete run).
export interface VaultRun {
  slug: string;
  skill: string;
  state: string;
  branch: string;
  goal: string;
  repo_root: string;
  workspace_path: string;
  source_path: string;
  acceptance_ok?: boolean;
  acceptance_exit?: number;
  synced_at: string;
  // Only populated by fetchVaultRun (detail); list rows omit children.
  phases?: VaultPhase[];
  metrics?: VaultMetric[];
}

// VaultRunDetail is a VaultRun with its phases and metrics guaranteed to be
// present (the detail endpoint always returns arrays, never null).
export interface VaultRunDetail extends VaultRun {
  phases: VaultPhase[];
  metrics: VaultMetric[];
}

// VaultFilter narrows the run listing. Empty / undefined = no filter.
export interface VaultFilter {
  skill?: string;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, authHeaders({ signal }));
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as T;
}

export async function fetchVaultRuns(
  filter: VaultFilter = {},
  signal?: AbortSignal,
): Promise<VaultRun[]> {
  const params = new URLSearchParams();
  if (filter.skill) params.set("skill", filter.skill);
  const qs = params.toString();
  const path = qs ? `/vault/runs?${qs}` : "/vault/runs";
  const body = await getJSON<{ runs: VaultRun[] }>(path, signal);
  return body.runs ?? [];
}

// fetchVaultRun loads one run by slug with phases and metrics attached. Slugs
// are date-prefixed and contain no slashes, so they ride in one path segment
// (encodeURIComponent is still applied for safety against odd characters).
export async function fetchVaultRun(
  slug: string,
  signal?: AbortSignal,
): Promise<VaultRunDetail> {
  const run = await getJSON<VaultRun>(
    `/vault/runs/${encodeURIComponent(slug)}`,
    signal,
  );
  return {
    ...run,
    phases: run.phases ?? [],
    metrics: run.metrics ?? [],
  };
}
