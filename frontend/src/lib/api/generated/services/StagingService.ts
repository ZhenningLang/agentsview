/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { StagingCandidatesOutput } from '../models/StagingCandidatesOutput';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class StagingService {
  /**
   * List staging memory candidates (the 备选池)
   * @returns StagingCandidatesOutput OK
   * @throws ApiError
   */
  public static getApiV1StagingCandidates({
    scope,
    limit,
  }: {
    /**
     * Filter by origin scope: user | project (empty = all)
     */
    scope?: string,
    /**
     * Max candidates to return (0 = all)
     */
    limit?: number,
  }): CancelablePromise<StagingCandidatesOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/staging/candidates',
      query: {
        'scope': scope,
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
