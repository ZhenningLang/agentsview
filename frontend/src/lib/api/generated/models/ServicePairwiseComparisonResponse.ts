/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ServicePairwiseDelta } from './ServicePairwiseDelta';
import type { ServicePairwiseSide } from './ServicePairwiseSide';
import type { ServicePairwiseUsageMetrics } from './ServicePairwiseUsageMetrics';
export type ServicePairwiseComparisonResponse = {
  deltas: ServicePairwiseDelta;
  left: ServicePairwiseSide;
  leftMetrics: ServicePairwiseUsageMetrics;
  right: ServicePairwiseSide;
  rightMetrics: ServicePairwiseUsageMetrics;
};
