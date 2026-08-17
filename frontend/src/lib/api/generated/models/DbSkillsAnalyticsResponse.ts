/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSkillTrendEntry } from './DbSkillTrendEntry';
import type { DbSkillUsage } from './DbSkillUsage';
export type DbSkillsAnalyticsResponse = {
  by_skill: Array<DbSkillUsage> | null;
  distinct_skills: number;
  total_skill_calls: number;
  trend: Array<DbSkillTrendEntry> | null;
};
