/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type PairwiseUsageMetrics = {
  cacheCreationTokens: number;
  cacheReadTokens: number;
  costPerSession?: number;
  hasCost: boolean;
  inputTokens: number;
  outputTokens: number;
  sessionCount: number;
  tokensPerSession?: number;
  totalCost: number;
  totalTokens: number;
  unpricedModels?: Array<string> | null;
};
