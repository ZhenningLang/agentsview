/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbDailyUsageEntry } from './DbDailyUsageEntry';
import type { DbUsageSessionCounts } from './DbUsageSessionCounts';
import type { DbUsageTotals } from './DbUsageTotals';
import type { ServiceAgentTotal } from './ServiceAgentTotal';
import type { ServiceCacheStats } from './ServiceCacheStats';
import type { ServiceComparison } from './ServiceComparison';
import type { ServiceMachineTotal } from './ServiceMachineTotal';
import type { ServiceModelTotal } from './ServiceModelTotal';
import type { ServiceProjectTotal } from './ServiceProjectTotal';
export type ServiceUsageSummaryResult = {
  agentTotals: Array<ServiceAgentTotal> | null;
  cacheStats: ServiceCacheStats;
  comparison?: ServiceComparison;
  daily: Array<DbDailyUsageEntry> | null;
  from: string;
  machineTotals: Array<ServiceMachineTotal> | null;
  modelTotals: Array<ServiceModelTotal> | null;
  projectTotals: Array<ServiceProjectTotal> | null;
  sessionCounts: DbUsageSessionCounts;
  to: string;
  totals: DbUsageTotals;
};
