/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbVaultMetric } from './DbVaultMetric';
import type { DbVaultPhase } from './DbVaultPhase';
export type DbVaultRun = {
  acceptance_exit?: number;
  acceptance_ok?: boolean;
  branch: string;
  goal: string;
  metrics?: Array<DbVaultMetric> | null;
  phases?: Array<DbVaultPhase> | null;
  repo_root: string;
  skill: string;
  slug: string;
  source_path: string;
  state: string;
  synced_at: string;
  workspace_path: string;
};
