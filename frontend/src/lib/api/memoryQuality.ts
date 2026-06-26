import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

export interface MemoryQualityTelemetryRow {
  schema: string;
  ts: string;
  event: string;
  source: string;
  duration_ms?: number;
  status?: string;
  reason?: string;
  candidate_count?: number;
  candidate_written?: boolean;
  candidate_id?: string;
  platform?: string;
  skipped_reasons?: Record<string, number>;
  capsule_count?: number;
  injected?: boolean;
  memory_injected?: boolean;
  prompt_chars?: number;
  context_chars?: number;
  route?: string;
  hit_count?: number;
  scores?: number[];
  fallback_triggered?: boolean;
  fallback_reason?: string;
}

export interface MemoryQualitySummary {
  telemetry: {
    capture_attempts: number;
    candidate_count: number;
    capture_written: number;
    injection_count: number;
    recall_count: number;
    recall_hit_count: number;
    fallback_count: number;
    scores: number[];
  };
  extract: {
    sessions_scanned: number;
    candidate_count: number;
    written: number;
    deduped: number;
    rejected: number;
    drift_refused: number;
    llm_duration_ms: number;
    llm_call_count: number;
    provider_usage: Record<string, number>;
    llm_usage?: {
      prompt_tokens?: number;
      completion_tokens?: number;
      total_tokens?: number;
    };
    llm_cost?: {
      currency?: string;
      amount?: string;
    };
  };
  consolidate: {
    candidate_count: number;
    add_count: number;
    update_count: number;
    skip_count: number;
    committed: number;
    resynced: number;
    llm_duration_ms: number;
    llm_call_count: number;
    provider_usage: Record<string, number>;
    llm_usage?: {
      prompt_tokens?: number;
      completion_tokens?: number;
      total_tokens?: number;
    };
    llm_cost?: {
      currency?: string;
      amount?: string;
    };
  };
  telemetry_rows: MemoryQualityTelemetryRow[];
}

export async function fetchMemoryQuality(
  limit = 50,
  signal?: AbortSignal,
): Promise<MemoryQualitySummary> {
  const res = await fetch(
    `${getBase()}/memory/quality?limit=${limit}`,
    authHeaders({ signal }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  const body = (await res.json()) as Partial<MemoryQualitySummary>;
  return {
    telemetry: {
      capture_attempts: body.telemetry?.capture_attempts ?? 0,
      candidate_count: body.telemetry?.candidate_count ?? 0,
      capture_written: body.telemetry?.capture_written ?? 0,
      injection_count: body.telemetry?.injection_count ?? 0,
      recall_count: body.telemetry?.recall_count ?? 0,
      recall_hit_count: body.telemetry?.recall_hit_count ?? 0,
      fallback_count: body.telemetry?.fallback_count ?? 0,
      scores: body.telemetry?.scores ?? [],
    },
    extract: {
      sessions_scanned: body.extract?.sessions_scanned ?? 0,
      candidate_count: body.extract?.candidate_count ?? 0,
      written: body.extract?.written ?? 0,
      deduped: body.extract?.deduped ?? 0,
      rejected: body.extract?.rejected ?? 0,
      drift_refused: body.extract?.drift_refused ?? 0,
      llm_duration_ms: body.extract?.llm_duration_ms ?? 0,
      llm_call_count: body.extract?.llm_call_count ?? 0,
      provider_usage: body.extract?.provider_usage ?? {},
      llm_usage: body.extract?.llm_usage,
      llm_cost: body.extract?.llm_cost,
    },
    consolidate: {
      candidate_count: body.consolidate?.candidate_count ?? 0,
      add_count: body.consolidate?.add_count ?? 0,
      update_count: body.consolidate?.update_count ?? 0,
      skip_count: body.consolidate?.skip_count ?? 0,
      committed: body.consolidate?.committed ?? 0,
      resynced: body.consolidate?.resynced ?? 0,
      llm_duration_ms: body.consolidate?.llm_duration_ms ?? 0,
      llm_call_count: body.consolidate?.llm_call_count ?? 0,
      provider_usage: body.consolidate?.provider_usage ?? {},
      llm_usage: body.consolidate?.llm_usage,
      llm_cost: body.consolidate?.llm_cost,
    },
    telemetry_rows: body.telemetry_rows ?? [],
  };
}
