/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type DbSkill = {
  catalog_path: string;
  catalog_present: boolean;
  description: string;
  description_tokens: number;
  domain: string;
  file_present: boolean;
  frontmatter_name: string;
  health_error_count: number;
  invocation_count: number;
  migration_canonical?: string;
  migration_state?: string;
  name: string;
  prompt?: string;
  prompt_tokens: number;
  resolved_path: string;
  role: string;
  source_mtime: number;
  synced_at: string;
  tokenizer: string;
  total_prompt_tokens: number;
};
