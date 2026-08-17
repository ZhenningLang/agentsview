/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbToolAgentBreakdown } from './DbToolAgentBreakdown';
import type { DbToolCategoryCount } from './DbToolCategoryCount';
import type { DbToolTrendEntry } from './DbToolTrendEntry';
export type DbToolsAnalyticsResponse = {
  by_agent: Array<DbToolAgentBreakdown> | null;
  by_category: Array<DbToolCategoryCount> | null;
  total_calls: number;
  trend: Array<DbToolTrendEntry> | null;
};
