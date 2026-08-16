/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { PairwiseDelta } from './PairwiseDelta';
import type { PairwiseSide } from './PairwiseSide';
import type { PairwiseUsageMetrics } from './PairwiseUsageMetrics';
export type PairwiseComparisonResponse = {
  deltas: PairwiseDelta;
  left: PairwiseSide;
  leftMetrics: PairwiseUsageMetrics;
  right: PairwiseSide;
  rightMetrics: PairwiseUsageMetrics;
};
