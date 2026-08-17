/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbHeatmapEntry } from './DbHeatmapEntry';
import type { DbHeatmapLevels } from './DbHeatmapLevels';
export type DbHeatmapResponse = {
  entries: Array<DbHeatmapEntry> | null;
  entries_from: string;
  levels: DbHeatmapLevels;
  metric: string;
};
