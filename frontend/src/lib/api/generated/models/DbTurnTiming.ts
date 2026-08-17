/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbCallTiming } from './DbCallTiming';
export type DbTurnTiming = {
  calls: Array<DbCallTiming> | null;
  duration_ms: number | null;
  message_id: number;
  ordinal: number;
  primary_category: string;
  started_at: string;
};
