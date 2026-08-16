/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type MemoryFeedbackInputBody = {
  /**
   * Optional optimistic-concurrency sha
   */
  base_sha?: string;
  /**
   * Free-text feedback comment
   */
  comment: string;
  /**
   * Feedback status: pending, handled, or empty
   */
  status: string;
  /**
   * Feedback vote: up, down, or empty
   */
  vote: string;
};
