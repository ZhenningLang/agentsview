/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { MemoryRecallInputBody } from '../models/MemoryRecallInputBody';
import type { SearchMemoryRecallResponse } from '../models/SearchMemoryRecallResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class MemoryRecallService {
  /**
   * Recall memory notes
   * @returns SearchMemoryRecallResponse OK
   * @throws ApiError
   */
  public static postApiV1MemoryRecall({
    requestBody,
  }: {
    requestBody: MemoryRecallInputBody,
  }): CancelablePromise<SearchMemoryRecallResponse> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/memory/recall',
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
