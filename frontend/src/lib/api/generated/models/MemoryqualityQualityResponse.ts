/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { MemoryqualityConsolidateSummary } from './MemoryqualityConsolidateSummary';
import type { MemoryqualityExtractSummary } from './MemoryqualityExtractSummary';
import type { MemoryqualityTelemetryRecord } from './MemoryqualityTelemetryRecord';
import type { MemoryqualityTelemetrySummary } from './MemoryqualityTelemetrySummary';
export type MemoryqualityQualityResponse = {
  consolidate: MemoryqualityConsolidateSummary;
  extract: MemoryqualityExtractSummary;
  telemetry: MemoryqualityTelemetrySummary;
  telemetry_rows: Array<MemoryqualityTelemetryRecord> | null;
};
