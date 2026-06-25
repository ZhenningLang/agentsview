import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// MemoryBackupStatus mirrors the server memoryBackupStatusResponse: the linked
// private backup repo full name and whether it has been validated/claimed.
export interface MemoryBackupStatus {
  repo: string;
  linked: boolean;
}

// MemoryBackupConnectResult mirrors the server connectMemoryBackupResponse.
export interface MemoryBackupConnectResult {
  repo: string;
  // outcome is "created" or "linked_existing".
  outcome: string;
  private: boolean;
  marker_written: boolean;
  linked: boolean;
}

// fetchMemoryBackupStatus loads the persisted backup link status (repo +
// linked). It is read-only and safe to call on settings mount.
export async function fetchMemoryBackupStatus(
  signal?: AbortSignal,
): Promise<MemoryBackupStatus> {
  const res = await fetch(
    `${getBase()}/config/memory-backup`,
    authHeaders({ signal }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as MemoryBackupStatus;
}

// connectMemoryBackup validates/creates/claims the PRIVATE backup repo via the
// locally authenticated gh CLI (server-side). It never pushes — Phase 05 owns
// the backup. The optional markerContent lets the client embed a timestamp the
// server does not read a clock for.
export async function connectMemoryBackup(
  namespaceOrUrl: string,
  markerContent?: string,
  signal?: AbortSignal,
): Promise<MemoryBackupConnectResult> {
  const res = await fetch(
    `${getBase()}/config/memory-backup/connect`,
    authHeaders({
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        namespace_or_url: namespaceOrUrl,
        marker_content: markerContent ?? "",
      }),
      signal,
    }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as MemoryBackupConnectResult;
}
