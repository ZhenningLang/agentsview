import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// ConsolidateDecision mirrors consolidate.DecisionRecord: one candidate's LLM
// action plus the dotfiles safety script's result line. A rejected candidate
// keeps a `skip <id> ...` result so the UI can show why it was not written.
export interface ConsolidateDecision {
  candidate_id: string;
  action: string;
  note_id?: string;
  reason?: string;
  result?: string;
}

// ConsolidateRun mirrors consolidate.RunRecord: one background consolidation
// cycle's audit entry.
export interface ConsolidateRun {
  started_at: string;
  finished_at?: string;
  candidate_count: number;
  decisions?: ConsolidateDecision[];
  script_exit_code?: number;
  script_errors?: string[];
  committed: boolean;
  resynced: boolean;
  skipped?: boolean;
  note?: string;
  error?: string;
}

export interface ConsolidateAudit {
  enabled: boolean;
  // available is true when runtime enable/disable is wired (a running worker
  // controller exists). When false, only config/env can arm the worker.
  available: boolean;
  records: ConsolidateRun[];
}

// ConsolidateEnableResult is the response from toggling the worker.
export interface ConsolidateEnableResult {
  enabled: boolean;
  available: boolean;
}

// fetchConsolidateAudit loads the consolidation run history newest-first. It is
// read-only: the worker runs as a background timer in the agentsview process.
export async function fetchConsolidateAudit(
  limit = 50,
  signal?: AbortSignal,
): Promise<ConsolidateAudit> {
  const res = await fetch(
    `${getBase()}/consolidate/audit?limit=${limit}`,
    authHeaders({ signal }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as ConsolidateAudit;
}

// setConsolidateEnabled arms or disarms the background consolidation worker.
// Enabling persists the choice and fires one immediate cycle server-side, so
// "开启后自动跑" holds without a restart (locked decision A2).
export async function setConsolidateEnabled(
  enabled: boolean,
  signal?: AbortSignal,
): Promise<ConsolidateEnableResult> {
  const res = await fetch(
    `${getBase()}/consolidate/enable`,
    authHeaders({
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled }),
      signal,
    }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as ConsolidateEnableResult;
}
