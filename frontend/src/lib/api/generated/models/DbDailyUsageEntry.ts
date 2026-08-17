/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbAgentBreakdown } from './DbAgentBreakdown';
import type { DbMachineBreakdown } from './DbMachineBreakdown';
import type { DbModelBreakdown } from './DbModelBreakdown';
import type { DbProjectBreakdown } from './DbProjectBreakdown';
export type DbDailyUsageEntry = {
  agentBreakdowns?: Array<DbAgentBreakdown> | null;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  date: string;
  hasCost: boolean;
  inputTokens: number;
  machineBreakdowns?: Array<DbMachineBreakdown> | null;
  modelBreakdowns?: Array<DbModelBreakdown> | null;
  modelsUsed: Array<string> | null;
  outputTokens: number;
  projectBreakdowns?: Array<DbProjectBreakdown> | null;
  totalCost: number;
  unpricedModels?: Array<string> | null;
};
