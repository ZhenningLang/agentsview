/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbEnrichmentStatusReport } from '../models/DbEnrichmentStatusReport';
import type { EnrichJobState } from '../models/EnrichJobState';
import type { LlmBalanceResponse } from '../models/LlmBalanceResponse';
import type { LlmConfigPatch } from '../models/LlmConfigPatch';
import type { LlmEnrichRequest } from '../models/LlmEnrichRequest';
import type { LlmEnrichResponse } from '../models/LlmEnrichResponse';
import type { LlmTestResponse } from '../models/LlmTestResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class LlmService {
  /**
   * Get LLM provider balance
   * @returns LlmBalanceResponse OK
   * @throws ApiError
   */
  public static getApiV1LlmBalance(): CancelablePromise<LlmBalanceResponse> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/llm/balance',
      errors: {
        400: `Bad Request`,
        401: `Unauthorized`,
        403: `Forbidden`,
        404: `Not Found`,
        409: `Conflict`,
        500: `Internal Server Error`,
        501: `Not Implemented`,
        502: `Bad Gateway`,
        503: `Service Unavailable`,
        504: `Gateway Timeout`,
      },
    });
  }
  /**
   * Trigger LLM enrichment
   * @returns LlmEnrichResponse OK
   * @throws ApiError
   */
  public static postApiV1LlmEnrich({
    requestBody,
  }: {
    requestBody: LlmEnrichRequest,
  }): CancelablePromise<LlmEnrichResponse> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/llm/enrich',
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
  /**
   * Get background LLM enrichment job state
   * @returns EnrichJobState OK
   * @throws ApiError
   */
  public static getApiV1LlmEnrichJob(): CancelablePromise<EnrichJobState> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/llm/enrich/job',
      errors: {
        400: `Bad Request`,
        401: `Unauthorized`,
        403: `Forbidden`,
        404: `Not Found`,
        409: `Conflict`,
        500: `Internal Server Error`,
        501: `Not Implemented`,
        502: `Bad Gateway`,
        503: `Service Unavailable`,
        504: `Gateway Timeout`,
      },
    });
  }
  /**
   * Start background LLM enrichment job
   * @returns EnrichJobState OK
   * @throws ApiError
   */
  public static postApiV1LlmEnrichStart(): CancelablePromise<EnrichJobState> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/llm/enrich/start',
      errors: {
        400: `Bad Request`,
        401: `Unauthorized`,
        403: `Forbidden`,
        404: `Not Found`,
        409: `Conflict`,
        500: `Internal Server Error`,
        501: `Not Implemented`,
        502: `Bad Gateway`,
        503: `Service Unavailable`,
        504: `Gateway Timeout`,
      },
    });
  }
  /**
   * Get LLM enrichment status
   * @returns DbEnrichmentStatusReport OK
   * @throws ApiError
   */
  public static getApiV1LlmEnrichStatus(): CancelablePromise<DbEnrichmentStatusReport> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/llm/enrich/status',
      errors: {
        400: `Bad Request`,
        401: `Unauthorized`,
        403: `Forbidden`,
        404: `Not Found`,
        409: `Conflict`,
        500: `Internal Server Error`,
        501: `Not Implemented`,
        502: `Bad Gateway`,
        503: `Service Unavailable`,
        504: `Gateway Timeout`,
      },
    });
  }
  /**
   * Stop background LLM enrichment job
   * @returns EnrichJobState OK
   * @throws ApiError
   */
  public static postApiV1LlmEnrichStop(): CancelablePromise<EnrichJobState> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/llm/enrich/stop',
      errors: {
        400: `Bad Request`,
        401: `Unauthorized`,
        403: `Forbidden`,
        404: `Not Found`,
        409: `Conflict`,
        500: `Internal Server Error`,
        501: `Not Implemented`,
        502: `Bad Gateway`,
        503: `Service Unavailable`,
        504: `Gateway Timeout`,
      },
    });
  }
  /**
   * Test LLM connection
   * @returns LlmTestResponse OK
   * @throws ApiError
   */
  public static postApiV1LlmTest({
    requestBody,
  }: {
    requestBody: LlmConfigPatch,
  }): CancelablePromise<LlmTestResponse> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/llm/test',
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
