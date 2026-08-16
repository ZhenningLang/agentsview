/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type SessionUsageResponse = {
  agent: string;
  cost_usd: number;
  has_cost: boolean;
  has_token_data: boolean;
  models: any[] | null;
  peak_context_tokens: number;
  project: string;
  breakdown_count: number;
  breakdown: any[] | null;
  rollup_cost_usd?: number;
  has_rollup_cost: boolean;
  rollup_subagent_count: number;
  server_running: boolean;
  session_id: string;
  total_output_tokens: number;
  unpriced_models: any[] | null;
};
