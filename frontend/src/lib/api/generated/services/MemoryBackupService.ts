/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { BackupPushEnableOutput } from '../models/BackupPushEnableOutput';
import type { BackupPushEnableRequest } from '../models/BackupPushEnableRequest';
import type { BackupPushStatusResponse } from '../models/BackupPushStatusResponse';
import type { ConnectMemoryBackupRequest } from '../models/ConnectMemoryBackupRequest';
import type { ConnectMemoryBackupResponse } from '../models/ConnectMemoryBackupResponse';
import type { MemoryBackupStatusResponse } from '../models/MemoryBackupStatusResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class MemoryBackupService {
  /**
   * Get memory backup link status
   * @returns MemoryBackupStatusResponse OK
   * @throws ApiError
   */
  public static getApiV1ConfigMemoryBackup(): CancelablePromise<MemoryBackupStatusResponse> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/config/memory-backup',
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
   * Validate/create/claim the memory backup repo
   * @returns ConnectMemoryBackupResponse OK
   * @throws ApiError
   */
  public static postApiV1ConfigMemoryBackupConnect({
    requestBody,
  }: {
    requestBody: ConnectMemoryBackupRequest,
  }): CancelablePromise<ConnectMemoryBackupResponse> {
    return __request(OpenAPI, {
      method: 'POST',
      url: '/api/v1/config/memory-backup/connect',
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
   * Enable or disable background backup push
   * @returns BackupPushEnableOutput OK
   * @throws ApiError
   */
  public static putApiV1ConfigMemoryBackupEnable({
    requestBody,
  }: {
    requestBody: BackupPushEnableRequest,
  }): CancelablePromise<BackupPushEnableOutput> {
    return __request(OpenAPI, {
      method: 'PUT',
      url: '/api/v1/config/memory-backup/enable',
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
   * Get background backup-push status
   * @returns BackupPushStatusResponse OK
   * @throws ApiError
   */
  public static getApiV1ConfigMemoryBackupPushStatus(): CancelablePromise<BackupPushStatusResponse> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/config/memory-backup/push-status',
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
}
