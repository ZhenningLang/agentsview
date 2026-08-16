/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbCallTiming } from './DbCallTiming';
import type { DbCategoryTotal } from './DbCategoryTotal';
import type { DbSessionSpeed } from './DbSessionSpeed';
import type { DbTurnTiming } from './DbTurnTiming';
export type DbSessionTiming = {
  by_category: Array<DbCategoryTotal> | null;
  running: boolean;
  session_id: string;
  slowest_call: DbCallTiming;
  speed: DbSessionSpeed;
  subagent_count: number;
  tool_call_count: number;
  tool_duration_ms: number;
  total_duration_ms: number;
  turn_count: number;
  turns: Array<DbTurnTiming> | null;
};
