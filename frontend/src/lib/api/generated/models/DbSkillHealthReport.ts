/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSkillHealth } from './DbSkillHealth';
export type DbSkillHealthReport = {
  by_check_type: Record<string, number>;
  by_severity: Record<string, number>;
  findings: Array<DbSkillHealth> | null;
  healthy_skills: number;
  total_skills: number;
};
