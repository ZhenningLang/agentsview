/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DbSkill } from '../models/DbSkill';
import type { DbSkillHealthReport } from '../models/DbSkillHealthReport';
import type { DbSkillTokenCostReport } from '../models/DbSkillTokenCostReport';
import type { SkillsListOutput } from '../models/SkillsListOutput';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class SkillsService {
  /**
   * List skills
   * @returns SkillsListOutput OK
   * @throws ApiError
   */
  public static getApiV1Skills({
    domain,
    role,
  }: {
    /**
     * Filter by domain
     */
    domain?: string,
    /**
     * Filter by role
     */
    role?: string,
  }): CancelablePromise<SkillsListOutput> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/skills',
      query: {
        'domain': domain,
        'role': role,
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
   * Skill static token cost
   * @returns DbSkillTokenCostReport OK
   * @throws ApiError
   */
  public static getApiV1SkillsCost(): CancelablePromise<DbSkillTokenCostReport> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/skills/cost',
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
   * Skill catalog health
   * @returns DbSkillHealthReport OK
   * @throws ApiError
   */
  public static getApiV1SkillsHealth({
    skill,
    checkType,
    severity,
  }: {
    /**
     * Filter by skill name
     */
    skill?: string,
    /**
     * Filter by check type
     */
    checkType?: string,
    /**
     * Filter by severity
     */
    severity?: string,
  }): CancelablePromise<DbSkillHealthReport> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/skills/health',
      query: {
        'skill': skill,
        'check_type': checkType,
        'severity': severity,
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
   * Get one skill
   * @returns DbSkill OK
   * @throws ApiError
   */
  public static getApiV1SkillsName({
    name,
  }: {
    /**
     * Skill name
     */
    name: string,
  }): CancelablePromise<DbSkill> {
    return __request(OpenAPI, {
      method: 'GET',
      url: '/api/v1/skills/{name}',
      path: {
        'name': name,
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
