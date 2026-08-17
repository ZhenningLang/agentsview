/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type EnrichJobState = {
  balance_end?: string;
  balance_start?: string;
  completion_tokens: number;
  cost_currency?: string;
  cost_spent?: string;
  done_at?: string;
  embed_balance_end?: string;
  embed_balance_start?: string;
  embed_cost_currency?: string;
  embed_cost_spent?: string;
  embed_tokens: number;
  error?: string;
  failed: number;
  no_content: number;
  processed: number;
  prompt_tokens: number;
  running: boolean;
  skipped: number;
  source?: string;
  started_at?: string;
  succeeded: number;
  total: number;
};
