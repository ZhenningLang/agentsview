/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SynthesizeAuditOutput } from '../models/SynthesizeAuditOutput';
import type { SynthesizeEnableOutput } from '../models/SynthesizeEnableOutput';
import type { SynthesizeEnableRequest } from '../models/SynthesizeEnableRequest';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class SynthesizeService {
  /**
   * List topic-synthesis audit history
   * @returns SynthesizeAuditOutput OK
   * @throws ApiError
   */
  public static getApiV1SynthesizeAudit({
    limit,
  }: {
    /**
     * Max records to return, newest first (0 = all)
     */
    limit?: number,
  }): CancelablePromise<SynthesizeAuditOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/synthesize/audit',
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
   * Enable or disable background topic synthesis
   * @returns SynthesizeEnableOutput OK
   * @throws ApiError
   */
  public static putApiV1SynthesizeEnable({
    requestBody,
  }: {
    requestBody: SynthesizeEnableRequest,
  }): CancelablePromise<SynthesizeEnableOutput> {
    return __request(OpenAPI, {
      method: 'PUT',
      url: '/api/v1/synthesize/enable',
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
