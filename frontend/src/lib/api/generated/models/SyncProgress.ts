/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type SyncProgress = {
  phase: string;
  detail?: string;
  hint?: string;
  resync?: boolean;
  current_project?: string;
  projects_total: number;
  projects_done: number;
  sessions_total: number;
  sessions_done: number;
  messages_indexed: number;
};
