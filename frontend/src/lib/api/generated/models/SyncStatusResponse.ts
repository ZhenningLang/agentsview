/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SyncSyncStats } from './SyncSyncStats';
import type { SyncProgress } from './SyncProgress';
export type SyncStatusResponse = {
  last_sync: string;
  stats: SyncSyncStats;
  progress?: SyncProgress;
};
