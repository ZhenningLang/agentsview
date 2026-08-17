/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbVaultRun } from '../models/DbVaultRun';
import type { VaultRunsListOutput } from '../models/VaultRunsListOutput';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class VaultService {
  /**
   * List dev-workflow runs
   * @returns VaultRunsListOutput OK
   * @throws ApiError
   */
  public static getApiV1VaultRuns({
    skill,
  }: {
    /**
     * Filter by run skill (e.g. dev-long-run, dev-complete)
     */
    skill?: string,
  }): CancelablePromise<VaultRunsListOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/vault/runs',
      query: {
        'skill': skill,
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
   * Get one run with phases and metrics
   * @returns DbVaultRun OK
   * @throws ApiError
   */
  public static getApiV1VaultRunsSlug({
    slug,
  }: {
    /**
     * Run slug
     */
    slug: string,
  }): CancelablePromise<DbVaultRun> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/vault/runs/{slug}',
      path: {
        'slug': slug,
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
