/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ConsolidateAuditOutput } from '../models/ConsolidateAuditOutput';
import type { ConsolidateEnableOutput } from '../models/ConsolidateEnableOutput';
import type { ConsolidateEnableRequest } from '../models/ConsolidateEnableRequest';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class ConsolidateService {
  /**
   * List consolidation audit history
   * @returns ConsolidateAuditOutput OK
   * @throws ApiError
   */
  public static getApiV1ConsolidateAudit({
    limit,
  }: {
    /**
     * Max records to return, newest first (0 = all)
     */
    limit?: number,
  }): CancelablePromise<ConsolidateAuditOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/consolidate/audit',
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
  /**
   * Automatic consolidation is removed
   * @returns ConsolidateEnableOutput OK
   * @throws ApiError
   */
  public static putApiV1ConsolidateEnable({
    requestBody,
  }: {
    requestBody: ConsolidateEnableRequest,
  }): CancelablePromise<ConsolidateEnableOutput> {
    return __request(OpenAPI, {
      method: 'PUT',
      url: '/api/v1/consolidate/enable',
      body: requestBody,
      mediaType: 'application/json',
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
