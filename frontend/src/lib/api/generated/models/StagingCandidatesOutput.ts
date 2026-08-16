/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { StagingCandidate } from './StagingCandidate';
export type StagingCandidatesOutput = {
  available: boolean;
  by_scope: Record<string, number>;
  candidates: Array<StagingCandidate> | null;
  projects: Record<string, number>;
  total: number;
};
