/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbTrendBucket } from './DbTrendBucket';
import type { DbTrendSeries } from './DbTrendSeries';
export type DbTrendsTermsResponse = {
  buckets: Array<DbTrendBucket> | null;
  from: string;
  granularity: string;
  message_count: number;
  series: Array<DbTrendSeries> | null;
  to: string;
};
