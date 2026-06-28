import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

// ExtractAudit mirrors the backend extractAuditOutput. `enabled` reflects the
// live worker state when a controller is wired (available=true), else config.
export interface ExtractAudit {
  enabled: boolean;
  available: boolean;
  records: unknown[];
}

export interface ExtractEnableResult {
  enabled: boolean;
  available: boolean;
}

// fetchExtractAudit loads the extraction worker state (and run history). Used by
// the settings toggle to show whether the background extractor is armed.
export async function fetchExtractAudit(
  limit = 1,
  signal?: AbortSignal,
): Promise<ExtractAudit> {
  const res = await fetch(
    `${getBase()}/extract/audit?limit=${limit}`,
    authHeaders({ signal }),
  );
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as ExtractAudit;
}

// setExtractEnabled arms or disarms the background LLM extraction worker
// (PUT /extract/enable). It persists extract_enabled and flips the live
// controller; the worker then runs on its timer (interval via config/env).
export async function setExtractEnabled(
  enabled: boolean,
  signal?: AbortSignal,
): Promise<ExtractEnableResult> {
  const res = await fetch(
    `${getBase()}/extract/enable`,
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
  return (await res.json()) as ExtractEnableResult;
}
