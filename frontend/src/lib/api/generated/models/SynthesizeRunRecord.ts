/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SynthesizeRawRef } from './SynthesizeRawRef';
import type { SynthesizeTopicRecord } from './SynthesizeTopicRecord';
export type SynthesizeRunRecord = {
  canonical_write_count?: number;
  cluster_count: number;
  committed: boolean;
  conflict_count?: number;
  conflict_samples?: Array<string> | null;
  coverage_refs?: Array<SynthesizeRawRef> | null;
  dry_run?: boolean;
  eligible_source_counts?: Record<string, number>;
  error?: string;
  failed_count?: number;
  finished_at?: string;
  note?: string;
  note_count: number;
  planned_canonical_count?: number;
  resynced: boolean;
  skipped?: boolean;
  skipped_count?: number;
  source_counts?: Record<string, number>;
  started_at: string;
  topics?: Array<SynthesizeTopicRecord> | null;
  write_count: number;
};
