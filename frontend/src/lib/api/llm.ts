import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";
import type { SearchResponse } from "./types.js";

export interface LLMBalanceResponse {
  supported: boolean;
  currency?: string;
  amount?: string;
  available: boolean;
}

export interface LLMEnrichRequest {
  project?: string;
  force?: boolean;
  limit?: number;
}

export interface LLMEnrichResponse {
  enriched: number;
  skipped: number;
  no_content: number;
  errors: number;
  candidates: number;
  elapsed_ms: number;
}

export interface LLMEnrichmentStatusReport {
  total: number;
  enriched: number;
  pending: number;
  skipped_too_short: number;
  no_content: number;
  errors: number;
  by_status: Record<string, number>;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, authHeaders({ signal }));
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as T;
}

async function postJSON<T>(
  path: string,
  body: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch(
    `${getBase()}${path}`,
    authHeaders({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
      signal,
    }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as T;
}

export function fetchBalance(signal?: AbortSignal): Promise<LLMBalanceResponse> {
  return getJSON<LLMBalanceResponse>("/llm/balance", signal);
}

export function triggerEnrich(
  request: LLMEnrichRequest = {},
  signal?: AbortSignal,
): Promise<LLMEnrichResponse> {
  return postJSON<LLMEnrichResponse>("/llm/enrich", request, signal);
}

export function fetchEnrichStatus(
  signal?: AbortSignal,
): Promise<LLMEnrichmentStatusReport> {
  return getJSON<LLMEnrichmentStatusReport>("/llm/enrich/status", signal);
}

export interface SemanticSearchResponse extends SearchResponse {
  disabled?: boolean;
}

export interface SemanticSearchStatusResponse {
  available: boolean;
}

export function fetchSemanticSearchStatus(
  signal?: AbortSignal,
): Promise<SemanticSearchStatusResponse> {
  return getJSON<SemanticSearchStatusResponse>("/search/semantic/status", signal);
}

export function semanticSearch(
  query: string,
  project?: string,
  k = 30,
  signal?: AbortSignal,
): Promise<SemanticSearchResponse> {
  const params = new URLSearchParams({ q: query, k: String(k) });
  if (project) params.set("project", project);
  return getJSON<SemanticSearchResponse>(`/search/semantic?${params.toString()}`, signal);
}
