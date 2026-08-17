/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ExtractCandidateRecord } from './ExtractCandidateRecord';
import type { ExtractLLMCost } from './ExtractLLMCost';
import type { ExtractLLMUsage } from './ExtractLLMUsage';
export type ExtractRunRecord = {
  candidate_count: number;
  candidates?: Array<ExtractCandidateRecord> | null;
  deduped: number;
  drift_refused: number;
  error?: string;
  finished_at?: string;
  llm_call_count?: number;
  llm_cost?: ExtractLLMCost;
  llm_duration_ms?: number;
  llm_usage?: ExtractLLMUsage;
  note?: string;
  provider_usage?: string;
  rejected: number;
  session_count: number;
  skipped?: boolean;
  staging_files: number;
  started_at: string;
  written: number;
};
