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

export interface LLMEnrichJobState {
  running: boolean;
  source?: string;
  processed: number;
  total: number;
  succeeded: number;
  no_content: number;
  failed: number;
  skipped: number;
  started_at?: string;
  done_at?: string;
  error?: string;
  prompt_tokens: number;
  completion_tokens: number;
  embed_tokens: number;
  cost_currency?: string;
  cost_spent?: string;
  balance_start?: string;
  balance_end?: string;
  embed_cost_currency?: string;
  embed_cost_spent?: string;
  embed_balance_start?: string;
  embed_balance_end?: string;
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

export interface LLMEmbedConfigResponse {
  base_url?: string;
  model?: string;
  has_api_key: boolean;
  api_key_preview?: string;
  balance_url?: string;
}

export interface LLMConfigResponse {
  enabled: boolean;
  base_url?: string;
  model?: string;
  reasoning_effort?: string;
  min_user_messages: number;
  reenrich_msg_delta: number;
  reenrich_idle_minutes: number;
  concurrency: number;
  periodic: boolean;
  balance_url?: string;
  has_api_key: boolean;
  api_key_preview?: string;
  embed: LLMEmbedConfigResponse;
}

export interface LLMProviderConfigResponse {
  enabled: boolean;
  base_url?: string;
  model?: string;
  reasoning_effort?: string;
  balance_url?: string;
  has_api_key: boolean;
  api_key_preview?: string;
}

export interface LLMProvidersResponse extends LLMConfigResponse {
  providers?: Record<string, LLMProviderConfigResponse>;
  usage?: Record<string, string>;
  usage_warnings?: string[];
}

export interface LLMEmbedConfigPayload {
  base_url?: string;
  api_key?: string;
  model?: string;
  balance_url?: string;
}

export interface LLMConfigPayload {
  enabled?: boolean;
  base_url?: string;
  api_key?: string;
  model?: string;
  reasoning_effort?: string;
  min_user_messages?: number;
  reenrich_msg_delta?: number;
  reenrich_idle_minutes?: number;
  concurrency?: number;
  periodic?: boolean;
  balance_url?: string;
  embed?: LLMEmbedConfigPayload;
}

export interface LLMProviderConfigPayload {
  enabled?: boolean;
  base_url?: string;
  api_key?: string;
  model?: string;
  reasoning_effort?: string;
  balance_url?: string;
}

export interface LLMProvidersPayload {
  providers?: Record<string, LLMProviderConfigPayload>;
  usage?: Record<string, string>;
  delete_providers?: string[];
}

export interface ConsolidateConfigResponse {
  enabled: boolean;
  interval: string;
}

export interface ConsolidateConfigPayload {
  interval?: string;
}

export interface LLMTestChannelResult {
  ok: boolean;
  disabled?: boolean;
  message: string;
}

export interface LLMTestResponse {
  chat: LLMTestChannelResult;
  embed: LLMTestChannelResult;
}

// Routing hints for POST /llm/test. `usage` tests a usage's effective resolved
// config; `provider` tests a stored named provider (its real secret); `channel`
// restricts to one transport. All optional — raw connection fields still work.
export interface LLMTestRequest extends LLMConfigPayload {
  usage?: string;
  provider?: string;
  channel?: "chat" | "embed";
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

async function patchJSON<T>(
  path: string,
  body: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch(
    `${getBase()}${path}`,
    authHeaders({
      method: "PATCH",
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

export function startEnrichJob(signal?: AbortSignal): Promise<LLMEnrichJobState> {
  return postJSON<LLMEnrichJobState>("/llm/enrich/start", {}, signal);
}

export function stopEnrichJob(signal?: AbortSignal): Promise<LLMEnrichJobState> {
  return postJSON<LLMEnrichJobState>("/llm/enrich/stop", {}, signal);
}

export function fetchEnrichJob(signal?: AbortSignal): Promise<LLMEnrichJobState> {
  return getJSON<LLMEnrichJobState>("/llm/enrich/job", signal);
}

export function fetchLLMConfig(signal?: AbortSignal): Promise<LLMConfigResponse> {
  return getJSON<LLMConfigResponse>("/config/llm", signal);
}

export function saveLLMConfig(
  payload: LLMConfigPayload,
  signal?: AbortSignal,
): Promise<LLMConfigResponse> {
  return postJSON<LLMConfigResponse>("/config/llm", payload, signal);
}

export function fetchLLMProviders(signal?: AbortSignal): Promise<LLMProvidersResponse> {
  return getJSON<LLMProvidersResponse>("/config/llm/providers", signal);
}

export function saveLLMProviders(
  payload: LLMProvidersPayload,
  signal?: AbortSignal,
): Promise<LLMProvidersResponse> {
  return patchJSON<LLMProvidersResponse>("/config/llm/providers", payload, signal);
}

export function fetchConsolidateConfig(
  signal?: AbortSignal,
): Promise<ConsolidateConfigResponse> {
  return getJSON<ConsolidateConfigResponse>("/config/consolidate", signal);
}

export function saveConsolidateConfig(
  payload: ConsolidateConfigPayload,
  signal?: AbortSignal,
): Promise<ConsolidateConfigResponse> {
  return patchJSON<ConsolidateConfigResponse>("/config/consolidate", payload, signal);
}

export function testLLMConnection(
  payload: LLMTestRequest = {},
  signal?: AbortSignal,
): Promise<LLMTestResponse> {
  return postJSON<LLMTestResponse>("/llm/test", payload, signal);
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
