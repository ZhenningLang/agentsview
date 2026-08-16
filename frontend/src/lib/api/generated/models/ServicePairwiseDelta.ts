/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ServicePairwiseDelta = {
  cacheCreationTokens: number;
  cacheReadTokens: number;
  costPerSession?: number;
  costRelativeChange?: number;
  hasCost: boolean;
  inputTokens: number;
  outputTokens: number;
  sessionCount: number;
  tokensPerSession?: number;
  tokensRelativeChange?: number;
  totalCost: number;
  totalTokens: number;
  unpricedModels?: Array<string> | null;
};
