/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSkill } from './DbSkill';
import type { DbSkillDomainCost } from './DbSkillDomainCost';
export type DbSkillTokenCostReport = {
  approximate: boolean;
  by_domain: Array<DbSkillDomainCost> | null;
  skills: Array<DbSkill> | null;
  tokenizer: string;
  total_skills: number;
  total_tokens: number;
};
