/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbTrendPoint } from './DbTrendPoint';
export type DbTrendSeries = {
  points: Array<DbTrendPoint> | null;
  term: string;
  total: number;
  variants: Array<string> | null;
};
