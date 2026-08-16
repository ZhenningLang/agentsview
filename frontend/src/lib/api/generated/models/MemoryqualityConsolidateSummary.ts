/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ConsolidateLLMCost } from './ConsolidateLLMCost';
import type { ConsolidateLLMUsage } from './ConsolidateLLMUsage';
export type MemoryqualityConsolidateSummary = {
  add_count: number;
  candidate_count: number;
  committed: number;
  llm_call_count: number;
  llm_cost?: ConsolidateLLMCost;
  llm_duration_ms: number;
  llm_usage?: ConsolidateLLMUsage;
  provider_usage: Record<string, number>;
  resynced: number;
  skip_count: number;
  update_count: number;
};
