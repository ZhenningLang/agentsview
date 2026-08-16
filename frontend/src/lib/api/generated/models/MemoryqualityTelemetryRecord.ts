/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type MemoryqualityTelemetryRecord = {
  candidate_count?: number;
  candidate_id?: string;
  candidate_written?: boolean;
  capsule_count?: number;
  context_chars?: number;
  duration_ms?: number;
  event: string;
  fallback_reason?: string;
  fallback_triggered?: boolean;
  hit_count?: number;
  injected?: boolean;
  memory_injected?: boolean;
  platform?: string;
  prompt_chars?: number;
  reason?: string;
  route?: string;
  schema: string;
  scores?: Array<number> | null;
  skipped_reasons?: Record<string, number>;
  source: string;
  status?: string;
  ts: string;
};
