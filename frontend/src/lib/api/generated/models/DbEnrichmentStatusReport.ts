/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type DbEnrichmentStatusReport = {
  by_status: Record<string, number>;
  enriched: number;
  errors: number;
  no_content: number;
  pending: number;
  skipped_too_short: number;
  total: number;
};
