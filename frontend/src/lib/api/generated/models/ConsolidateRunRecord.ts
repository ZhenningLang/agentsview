/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ConsolidateDecisionRecord } from './ConsolidateDecisionRecord';
import type { ConsolidateLLMCost } from './ConsolidateLLMCost';
import type { ConsolidateLLMUsage } from './ConsolidateLLMUsage';
export type ConsolidateRunRecord = {
  add_count?: number;
  candidate_count: number;
  committed: boolean;
  decisions?: Array<ConsolidateDecisionRecord> | null;
  delete_count?: number;
  drained_count?: number;
  error?: string;
  finished_at?: string;
  invalidate_count?: number;
  llm_call_count?: number;
  llm_cost?: ConsolidateLLMCost;
  llm_duration_ms?: number;
  llm_usage?: ConsolidateLLMUsage;
  note?: string;
  provider_usage?: string;
  resynced: boolean;
  script_errors?: Array<string> | null;
  script_exit_code?: number;
  skip_count?: number;
  skipped?: boolean;
  started_at: string;
  update_count?: number;
};
