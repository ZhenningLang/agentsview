/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { LlmEmbedConfigPatch } from './LlmEmbedConfigPatch';
export type LlmConfigPatch = {
  api_key?: string;
  balance_url?: string;
  base_url?: string;
  /**
   * (test only) Restrict to one channel: "chat" or "embed"
   */
  channel?: string;
  concurrency?: number;
  embed?: LlmEmbedConfigPatch;
  enabled?: boolean;
  min_user_messages?: number;
  model?: string;
  periodic?: boolean;
  /**
   * (test only) Resolve and test this named registry provider's stored secret
   */
  provider?: string;
  reasoning_effort?: string;
  reenrich_idle_minutes?: number;
  reenrich_msg_delta?: number;
  /**
   * (test only) Resolve and test this usage's effective config
   */
  usage?: string;
};
