/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbDistributionBucket } from './DbDistributionBucket';
export type DbSessionShapeResponse = {
  autonomy_distribution: Array<DbDistributionBucket> | null;
  count: number;
  duration_distribution: Array<DbDistributionBucket> | null;
  length_distribution: Array<DbDistributionBucket> | null;
};
