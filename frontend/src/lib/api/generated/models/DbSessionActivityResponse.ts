/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSessionActivityBucket } from './DbSessionActivityBucket';
export type DbSessionActivityResponse = {
  buckets: Array<DbSessionActivityBucket> | null;
  interval_seconds: number;
  total_messages: number;
};
