import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// Memory mirrors db.Memory: one user-memory note (a markdown file under the
// memory SSOT) with YAML frontmatter fields plus a body and sync metadata.
export interface Memory {
  rel_path: string;
  // Data source: "cross-agent" (the user-memory SSOT) or "cc-native"
  // (CC auto-memory scanned across project dirs).
  source: string;
  title: string;
  date: string;
  problem_type: string;
  type: string;
  status: string;
  origin_session: string;
  // Project a note belongs to ("" = the General bucket: user-global or
  // cross-project notes). Drives the project facet/grouping.
  origin_project: string;
  feedback_vote: string;
  feedback_comment: string;
  feedback_status: string;
  body?: string;
  body_tokens: number;
  source_mtime: number;
  synced_at: string;
}

// MemoryFilter narrows the listing. Empty / undefined fields = no filter. q is
// a full-text query over the note body.
export interface MemoryFilter {
  source?: string;
  problem_type?: string;
  type?: string;
  status?: string;
  origin_session?: string;
  origin_project?: string;
  feedback_vote?: string;
  feedback_comment?: string;
  feedback_status?: string;
  q?: string;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, authHeaders({ signal }));
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as T;
}

async function sendJSON<T>(
  method: "PUT" | "POST",
  path: string,
  body: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch(
    `${getBase()}${path}`,
    authHeaders({
      method,
      signal,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as T;
}

export async function fetchMemories(
  filter: MemoryFilter = {},
  signal?: AbortSignal,
): Promise<Memory[]> {
  const params = new URLSearchParams();
  if (filter.source) params.set("source", filter.source);
  if (filter.problem_type) params.set("problem_type", filter.problem_type);
  if (filter.type) params.set("type", filter.type);
  if (filter.status) params.set("status", filter.status);
  if (filter.origin_session) params.set("origin_session", filter.origin_session);
  if (filter.origin_project) params.set("origin_project", filter.origin_project);
  if (filter.feedback_vote) params.set("feedback_vote", filter.feedback_vote);
  if (filter.feedback_status) params.set("feedback_status", filter.feedback_status);
  if (filter.q) params.set("q", filter.q);
  const qs = params.toString();
  const path = qs ? `/memories?${qs}` : "/memories";
  const body = await getJSON<{ memories: Memory[] }>(path, signal);
  return body.memories ?? [];
}

// fetchMemory loads one note by its rel_path. The backend keys notes by
// rel_path which contains slashes, so the path is URL-safe base64 encoded
// (matching the server's base64.RawURLEncoding decode).
export function fetchMemory(
  relPath: string,
  signal?: AbortSignal,
): Promise<Memory> {
  return getJSON<Memory>(`/memories/${encodeMemoryPath(relPath)}`, signal);
}

// MemoryRaw is the verbatim on-disk file content plus its sha256. The editor
// loads this (not the DB-parsed Memory) so it can round-trip untracked
// frontmatter keys and obtain a base_sha that matches the write gate.
export interface MemoryRaw {
  content: string;
  sha: string;
}

// fetchMemoryRaw loads a note's raw file content and sha for editing.
export function fetchMemoryRaw(
  relPath: string,
  signal?: AbortSignal,
): Promise<MemoryRaw> {
  return getJSON<MemoryRaw>(
    `/memories/${encodeMemoryPath(relPath)}/raw`,
    signal,
  );
}

// putMemory writes back the full reconstructed file content. base_sha is the
// sha the editor read at edit time; a stale base yields a 409 (ApiError with
// status 409) which the caller must surface, never silently drop.
export function putMemory(
  relPath: string,
  content: string,
  baseSHA: string,
  signal?: AbortSignal,
): Promise<{ sha: string }> {
  return sendJSON<{ sha: string }>(
    "PUT",
    `/memories/${encodeMemoryPath(relPath)}`,
    { content, base_sha: baseSHA },
    signal,
  );
}

export interface MemoryFeedbackInput {
  vote: "up" | "down" | "";
  comment: string;
  status: "pending" | "handled" | "";
  base_sha?: string;
}

export function setMemoryFeedback(
  relPath: string,
  feedback: MemoryFeedbackInput,
  signal?: AbortSignal,
): Promise<{ sha: string }> {
  return sendJSON<{ sha: string }>(
    "POST",
    `/memories/${encodeMemoryPath(relPath)}/feedback`,
    feedback,
    signal,
  );
}

// MemoryHistoryEntry mirrors memory.HistoryEntry: one git commit touching the
// note, newest first.
export interface MemoryHistoryEntry {
  commit: string;
  date: string;
  message: string;
}

// fetchMemoryHistory lists the git history for a note (empty when the memory
// dir is not a git repo or the file was never committed).
export async function fetchMemoryHistory(
  relPath: string,
  signal?: AbortSignal,
): Promise<MemoryHistoryEntry[]> {
  const body = await getJSON<{ history: MemoryHistoryEntry[] }>(
    `/memories/${encodeMemoryPath(relPath)}/history`,
    signal,
  );
  return body.history ?? [];
}

// fetchMemoryAtCommit returns the note's content at a specific commit, for the
// history diff view.
export async function fetchMemoryAtCommit(
  relPath: string,
  commit: string,
  signal?: AbortSignal,
): Promise<string> {
  const body = await getJSON<{ content: string }>(
    `/memories/${encodeMemoryPath(relPath)}/history/${encodeURIComponent(commit)}`,
    signal,
  );
  return body.content ?? "";
}

// revertMemory restores the note to a past commit. base_sha guards against a
// concurrent on-disk change between viewing and reverting (409 on mismatch).
export function revertMemory(
  relPath: string,
  commit: string,
  baseSHA: string,
  signal?: AbortSignal,
): Promise<{ sha: string }> {
  return sendJSON<{ sha: string }>(
    "POST",
    `/memories/${encodeMemoryPath(relPath)}/revert`,
    { commit, base_sha: baseSHA },
    signal,
  );
}

// encodeMemoryPath produces URL-safe base64 without padding, matching Go's
// base64.RawURLEncoding so the server can round-trip the rel_path.
function encodeMemoryPath(relPath: string): string {
  const bytes = new TextEncoder().encode(relPath);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}
