/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSearchResult } from './DbSearchResult';
export type SearchResponse = {
  count: number;
  next: number;
  query: string;
  results: Array<DbSearchResult> | null;
};
