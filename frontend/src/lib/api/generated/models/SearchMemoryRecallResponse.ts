/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SearchMemoryRecallHit } from './SearchMemoryRecallHit';
export type SearchMemoryRecallResponse = {
  count: number;
  disabled: boolean;
  hits: Array<SearchMemoryRecallHit> | null;
  query: string;
};
