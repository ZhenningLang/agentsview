/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSearchResult } from './DbSearchResult';
export type SearchSemanticResponse = {
  count: number;
  disabled: boolean;
  query: string;
  results: Array<DbSearchResult> | null;
};
