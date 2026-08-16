/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSpeedConcurrencyPoint } from './DbSpeedConcurrencyPoint';
import type { DbSpeedTrendSeries } from './DbSpeedTrendSeries';
export type DbSpeedTrendResponse = {
  bucket_sec: number;
  concurrency: Array<DbSpeedConcurrencyPoint> | null;
  group_by: string;
  series: Array<DbSpeedTrendSeries> | null;
  since: string;
  until: string;
};
