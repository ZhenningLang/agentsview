/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ExtractAuditOutput } from '../models/ExtractAuditOutput';
import type { ExtractEnableOutput } from '../models/ExtractEnableOutput';
import type { ExtractEnableRequest } from '../models/ExtractEnableRequest';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class ExtractService {
  /**
   * List extraction audit history
   * @returns ExtractAuditOutput OK
   * @throws ApiError
   */
  public static getApiV1ExtractAudit({
    limit,
  }: {
    /**
     * Max records to return, newest first (0 = all)
     */
    limit?: number,
  }): CancelablePromise<ExtractAuditOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/extract/audit',
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
   * Automatic extraction is removed
   * @returns ExtractEnableOutput OK
   * @throws ApiError
   */
  public static putApiV1ExtractEnable({
    requestBody,
  }: {
    requestBody: ExtractEnableRequest,
  }): CancelablePromise<ExtractEnableOutput> {
    return __request(OpenAPI, {
      method: 'PUT',
      url: '/api/v1/extract/enable',
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
