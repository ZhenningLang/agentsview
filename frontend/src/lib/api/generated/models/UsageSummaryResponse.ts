/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { AgentTotal } from './AgentTotal';
import type { CacheStats } from './CacheStats';
import type { Comparison } from './Comparison';
import type { DbDailyUsageEntry } from './DbDailyUsageEntry';
import type { DbUsageSessionCounts } from './DbUsageSessionCounts';
import type { DbUsageTotals } from './DbUsageTotals';
import type { MachineTotal } from './MachineTotal';
import type { ModelTotal } from './ModelTotal';
import type { ProjectTotal } from './ProjectTotal';
export type UsageSummaryResponse = {
  agentTotals: Array<AgentTotal> | null;
  cacheStats: CacheStats;
  comparison?: Comparison;
  daily: Array<DbDailyUsageEntry> | null;
  from: string;
  machineTotals: Array<MachineTotal> | null;
  modelTotals: Array<ModelTotal> | null;
  projectTotals: Array<ProjectTotal> | null;
  sessionCounts: DbUsageSessionCounts;
  to: string;
  totals: DbUsageTotals;
};
