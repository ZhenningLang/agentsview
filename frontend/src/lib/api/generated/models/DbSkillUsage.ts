/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSkillAgentBreakdown } from './DbSkillAgentBreakdown';
import type { DbSkillProjectBreakdown } from './DbSkillProjectBreakdown';
export type DbSkillUsage = {
  agent_breakdown: Array<DbSkillAgentBreakdown> | null;
  call_count: number;
  last_used_at: string;
  pct: number;
  project_breakdown: Array<DbSkillProjectBreakdown> | null;
  session_count: number;
  skill_name: string;
};
