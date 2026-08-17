/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SynthesizeRawRef } from './SynthesizeRawRef';
export type SynthesizeTopicRecord = {
  covered_refs?: Array<SynthesizeRawRef> | null;
  error?: string;
  rel_path?: string;
  result: string;
  skipped?: boolean;
  source_ids: Array<string> | null;
  title: string;
};
