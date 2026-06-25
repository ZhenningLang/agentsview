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

// BackupPushStatus mirrors the server backupPushStatusResponse: the live armed
// state plus the latest cycle outcome (last success / last error) so the UI can
// show a green/red indicator for the background backup-push worker.
export interface BackupPushStatus {
  enabled: boolean;
  available: boolean;
  repo?: string;
  last_attempt_at?: string;
  last_success_at?: string;
  last_error?: string;
  last_error_at?: string;
}

// fetchBackupPushStatus loads the background backup-push status. Read-only and
// fail-soft: callers treat a thrown error as "status unavailable".
export async function fetchBackupPushStatus(
  signal?: AbortSignal,
): Promise<BackupPushStatus> {
  const res = await fetch(
    `${getBase()}/config/memory-backup/push-status`,
    authHeaders({ signal }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as BackupPushStatus;
}

// setBackupPushEnabled arms or disarms the background backup-push worker.
// Enabling persists the choice and fires one immediate push cycle (server-side);
// it never pushes from the browser. Returns the resulting live state.
export async function setBackupPushEnabled(
  enabled: boolean,
  signal?: AbortSignal,
): Promise<{ enabled: boolean; available: boolean }> {
  const res = await fetch(
    `${getBase()}/config/memory-backup/enable`,
    authHeaders({
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled }),
      signal,
    }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as { enabled: boolean; available: boolean };
}
