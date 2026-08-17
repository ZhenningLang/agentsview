/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbVelocityBreakdown } from './DbVelocityBreakdown';
import type { DbVelocityOverview } from './DbVelocityOverview';
export type DbVelocityResponse = {
  by_agent: Array<DbVelocityBreakdown> | null;
  by_complexity: Array<DbVelocityBreakdown> | null;
  overall: DbVelocityOverview;
};
