/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type MemoryRecallInputBody = {
  /**
   * Prefer canonical memory rows and suppress covered raw duplicates
   */
  prefer_canonical?: boolean;
  /**
   * Filter by memory problem_type
   */
  problem_type?: string;
  /**
   * Recall query
   */
  query: string;
  /**
   * Filter by data source; explicit source filters bypass canonical suppression
   */
  source?: string;
  /**
   * Filter by memory status
   */
  status?: string;
  /**
   * Maximum number of hits
   */
  top_k: number;
};
