/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { LlmProviderConfigPatch } from './LlmProviderConfigPatch';
export type LlmProvidersPatch = {
  delete_providers?: Array<string> | null;
  providers?: Record<string, LlmProviderConfigPatch>;
  usage?: Record<string, string>;
  usage_model?: Record<string, string>;
};
