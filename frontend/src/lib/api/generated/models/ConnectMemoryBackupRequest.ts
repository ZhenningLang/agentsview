/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ConnectMemoryBackupRequest = {
  /**
   * Optional claim-marker body (client supplies any timestamp)
   */
  marker_content?: string;
  /**
   * Namespace, owner/name, or repo URL
   */
  namespace_or_url: string;
};
