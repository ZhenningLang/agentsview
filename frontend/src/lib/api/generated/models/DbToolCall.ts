/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbToolResultEvent } from './DbToolResultEvent';
export type DbToolCall = {
  category: string;
  input_json?: string;
  result_content?: string;
  result_content_length?: number;
  result_events?: Array<DbToolResultEvent> | null;
  skill_name?: string;
  subagent_session_id?: string;
  tool_name: string;
  tool_use_id?: string;
};
