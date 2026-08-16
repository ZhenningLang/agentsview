/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSessionUsageBreakdownEntry } from './DbSessionUsageBreakdownEntry';
export type SessionUsageResponse = {
  agent: string;
  breakdown: Array<DbSessionUsageBreakdownEntry> | null;
  breakdown_count: number;
  cost_usd: number;
  has_cost: boolean;
  has_rollup_cost: boolean;
  has_token_data: boolean;
  models: Array<string> | null;
  peak_context_tokens: number;
  project: string;
  rollup_cost_usd?: number;
  rollup_subagent_count: number;
  server_running: boolean;
  session_id: string;
  total_output_tokens: number;
  unpriced_models: Array<string> | null;
};
