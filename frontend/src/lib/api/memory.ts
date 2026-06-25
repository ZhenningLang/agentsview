import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// Memory mirrors db.Memory: one user-memory note (a markdown file under the
// memory SSOT) with YAML frontmatter fields plus a body and sync metadata.
export interface Memory {
  rel_path: string;
  title: string;
  date: string;
  problem_type: string;
  type: string;
  status: string;
  origin_session: string;
  body?: string;
  body_tokens: number;
  source_mtime: number;
  synced_at: string;
}

// MemoryFilter narrows the listing. Empty / undefined fields = no filter. q is
// a full-text query over the note body.
export interface MemoryFilter {
  problem_type?: string;
  type?: string;
  status?: string;
  origin_session?: string;
  q?: string;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, authHeaders({ signal }));
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
  if (filter.problem_type) params.set("problem_type", filter.problem_type);
  if (filter.type) params.set("type", filter.type);
  if (filter.status) params.set("status", filter.status);
  if (filter.origin_session) params.set("origin_session", filter.origin_session);
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
