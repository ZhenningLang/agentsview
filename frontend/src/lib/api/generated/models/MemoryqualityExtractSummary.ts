/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ExtractLLMCost } from './ExtractLLMCost';
import type { ExtractLLMUsage } from './ExtractLLMUsage';
export type MemoryqualityExtractSummary = {
  candidate_count: number;
  deduped: number;
  drift_refused: number;
  llm_call_count: number;
  llm_cost?: ExtractLLMCost;
  llm_duration_ms: number;
  llm_usage?: ExtractLLMUsage;
  provider_usage: Record<string, number>;
  rejected: number;
  sessions_scanned: number;
  written: number;
};
