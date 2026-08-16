/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { LlmEmbedConfigResponse } from './LlmEmbedConfigResponse';
import type { LlmProviderConfigResponse } from './LlmProviderConfigResponse';
export type LlmConfigResponse = {
  api_key_preview?: string;
  balance_url?: string;
  base_url?: string;
  concurrency: number;
  embed: LlmEmbedConfigResponse;
  enabled: boolean;
  has_api_key: boolean;
  min_user_messages: number;
  model?: string;
  periodic: boolean;
  providers?: Record<string, LlmProviderConfigResponse>;
  reasoning_effort?: string;
  reenrich_idle_minutes: number;
  reenrich_msg_delta: number;
  usage?: Record<string, string>;
  usage_model?: Record<string, string>;
  usage_warnings?: Array<string> | null;
};
