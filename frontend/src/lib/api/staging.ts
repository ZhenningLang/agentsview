import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// StagingCandidate mirrors the server's stagingCandidate: one raw memory
// candidate awaiting consolidation, tagged with its origin scope.
export interface StagingCandidate {
  id: string;
  summary: string;
  category: string;
  scope: string; // "user" | "project"
  origin_project: string;
  origin_session: string;
  created_at: string;
}

// StagingPool mirrors the server's stagingCandidatesOutput: the current 备选池
// (candidates queued for consolidation), split by origin scope.
export interface StagingPool {
  // available is true when a dotfiles root is configured (staging dir locatable).
  available: boolean;
  // total is the full pool size before any scope filter.
  total: number;
  // by_scope counts the full pool per origin scope (user/project).
  by_scope: Record<string, number>;
  // projects counts project-scoped candidates per origin_project.
  projects: Record<string, number>;
  candidates: StagingCandidate[];
}

// fetchStagingPool loads the staging candidate pool. The pool is read from disk
// on request (it is ephemeral — drained after consolidation — so not indexed in
// the DB). An optional scope filters the returned candidates; the counts always
// reflect the full pool.
export async function fetchStagingPool(
  scope: "" | "user" | "project" = "",
  limit = 0,
  signal?: AbortSignal,
): Promise<StagingPool> {
  const params = new URLSearchParams();
  if (scope) params.set("scope", scope);
  if (limit > 0) params.set("limit", String(limit));
  const qs = params.toString();
  const res = await fetch(
    `${getBase()}/staging/candidates${qs ? `?${qs}` : ""}`,
    authHeaders({ signal }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as StagingPool;
}
