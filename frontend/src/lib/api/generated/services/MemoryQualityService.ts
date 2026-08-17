/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { MemoryqualityQualityResponse } from '../models/MemoryqualityQualityResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class MemoryQualityService {
  /**
   * Get memory quality telemetry
   * @returns MemoryqualityQualityResponse OK
   * @throws ApiError
   */
  public static getApiV1MemoryQuality({
    limit,
  }: {
    /**
     * Max telemetry/audit rows to aggregate (0 = all)
     */
    limit?: number,
  }): CancelablePromise<MemoryqualityQualityResponse> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/memory/quality',
      query: {
        'limit': limit,
      },
      errors: {
        400: `Bad Request`,
        401: `Unauthorized`,
        403: `Forbidden`,
        404: `Not Found`,
        409: `Conflict`,
        422: `Unprocessable Entity`,
        500: `Internal Server Error`,
        501: `Not Implemented`,
        502: `Bad Gateway`,
        503: `Service Unavailable`,
        504: `Gateway Timeout`,
      },
    });
  }
}
