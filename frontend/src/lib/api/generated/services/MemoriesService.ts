/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbMemory } from '../models/DbMemory';
import type { MemoriesListOutput } from '../models/MemoriesListOutput';
import type { MemoryContentOutput } from '../models/MemoryContentOutput';
import type { MemoryFeedbackInputBody } from '../models/MemoryFeedbackInputBody';
import type { MemoryHistoryOutput } from '../models/MemoryHistoryOutput';
import type { MemoryPutInputBody } from '../models/MemoryPutInputBody';
import type { MemoryRawOutput } from '../models/MemoryRawOutput';
import type { MemoryRevertInputBody } from '../models/MemoryRevertInputBody';
import type { MemoryWriteOutput } from '../models/MemoryWriteOutput';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class MemoriesService {
  /**
   * List user-memory notes
   * @returns MemoriesListOutput OK
   * @throws ApiError
   */
  public static getApiV1Memories({
    source,
    problemType,
    type,
    status,
    originSession,
    originProject,
    feedbackVote,
    feedbackStatus,
    q,
  }: {
    /**
     * Filter by data source (assist-mem | cross-agent | cc-native | canonical)
     */
    source?: string,
    /**
     * Filter by frontmatter problem_type
     */
    problemType?: string,
    /**
     * Filter by frontmatter type
     */
    type?: string,
    /**
     * Filter by frontmatter status
     */
    status?: string,
    /**
     * Filter by originating session id
     */
    originSession?: string,
    /**
     * Filter by originating project ('' = General)
     */
    originProject?: string,
    /**
     * Filter by feedback vote (up | down)
     */
    feedbackVote?: string,
    /**
     * Filter by feedback status (pending | handled)
     */
    feedbackStatus?: string,
    /**
     * Full-text query over the note body
     */
    q?: string,
  }): CancelablePromise<MemoriesListOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/memories',
      query: {
        'source': source,
        'problem_type': problemType,
        'type': type,
        'status': status,
        'origin_session': originSession,
        'origin_project': originProject,
        'feedback_vote': feedbackVote,
        'feedback_status': feedbackStatus,
        'q': q,
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
   * Delete one memory note
   * @returns MemoryWriteOutput OK
   * @throws ApiError
   */
  public static deleteApiV1MemoriesPath({
    path,
    baseSha,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
    /**
     * sha256 of the content the editor read
     */
    baseSha?: string,
  }): CancelablePromise<MemoryWriteOutput> {
    return __request(OpenAPI, {
      method: 'DELETE',
      url: '/api/v1/memories/{path}',
      path: {
        'path': path,
      },
      query: {
        'base_sha': baseSha,
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
   * Get one memory note
   * @returns DbMemory OK
   * @throws ApiError
   */
  public static getApiV1MemoriesPath({
    path,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
  }): CancelablePromise<DbMemory> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/memories/{path}',
      path: {
        'path': path,
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
   * Write back one memory note
   * @returns MemoryWriteOutput OK
   * @throws ApiError
   */
  public static putApiV1MemoriesPath({
    path,
    requestBody,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
    requestBody: MemoryPutInputBody,
  }): CancelablePromise<MemoryWriteOutput> {
    return __request(OpenAPI, {
      method: 'PUT',
      url: '/api/v1/memories/{path}',
      path: {
        'path': path,
      },
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
   * Set feedback on a memory note
   * @returns MemoryWriteOutput OK
   * @throws ApiError
   */
  public static postApiV1MemoriesPathFeedback({
    path,
    requestBody,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
    requestBody: MemoryFeedbackInputBody,
  }): CancelablePromise<MemoryWriteOutput> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/memories/{path}/feedback',
      path: {
        'path': path,
      },
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
   * List git history for one memory note
   * @returns MemoryHistoryOutput OK
   * @throws ApiError
   */
  public static getApiV1MemoriesPathHistory({
    path,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
  }): CancelablePromise<MemoryHistoryOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/memories/{path}/history',
      path: {
        'path': path,
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
   * Get one memory note at a specific commit
   * @returns MemoryContentOutput OK
   * @throws ApiError
   */
  public static getApiV1MemoriesPathHistoryCommit({
    path,
    commit,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
    /**
     * Git commit hash
     */
    commit: string,
  }): CancelablePromise<MemoryContentOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/memories/{path}/history/{commit}',
      path: {
        'path': path,
        'commit': commit,
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
   * Get one memory note's raw on-disk content and sha
   * @returns MemoryRawOutput OK
   * @throws ApiError
   */
  public static getApiV1MemoriesPathRaw({
    path,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
  }): CancelablePromise<MemoryRawOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/memories/{path}/raw',
      path: {
        'path': path,
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
   * Revert one memory note to a commit
   * @returns MemoryWriteOutput OK
   * @throws ApiError
   */
  public static postApiV1MemoriesPathRevert({
    path,
    requestBody,
  }: {
    /**
     * URL-safe base64 of the memory rel_path
     */
    path: string,
    requestBody: MemoryRevertInputBody,
  }): CancelablePromise<MemoryWriteOutput> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/memories/{path}/revert',
      path: {
        'path': path,
      },
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
