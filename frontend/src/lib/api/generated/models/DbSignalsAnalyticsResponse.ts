/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSignalsAgentRow } from './DbSignalsAgentRow';
import type { DbSignalsContextHealth } from './DbSignalsContextHealth';
import type { DbSignalsProjectRow } from './DbSignalsProjectRow';
import type { DbSignalsToolHealth } from './DbSignalsToolHealth';
import type { DbSignalsTrendBucket } from './DbSignalsTrendBucket';
export type DbSignalsAnalyticsResponse = {
  avg_health_score: number | null;
  by_agent: Array<DbSignalsAgentRow> | null;
  by_project: Array<DbSignalsProjectRow> | null;
  context_health: DbSignalsContextHealth;
  grade_distribution: Record<string, number>;
  outcome_confidence_distribution: Record<string, number>;
  outcome_distribution: Record<string, number>;
  scored_sessions: number;
  tool_health: DbSignalsToolHealth;
  trend: Array<DbSignalsTrendBucket> | null;
  unscored_sessions: number;
};
