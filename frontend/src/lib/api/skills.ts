import { getBase, authHeaders, ApiError, responseErrorMessage } from "./runtime";

export interface Skill {
  name: string;
  catalog_path: string;
  resolved_path: string;
  domain: string;
  role: string;
  migration_state?: string;
  migration_canonical?: string;
  description: string;
  frontmatter_name: string;
  description_tokens: number;
  tokenizer: string;
  catalog_present: boolean;
  file_present: boolean;
  health_error_count: number;
  source_mtime: number;
  synced_at: string;
}

export interface SkillDomainCost {
  domain: string;
  skills: number;
  tokens: number;
}

export interface SkillTokenCostReport {
  total_skills: number;
  total_tokens: number;
  tokenizer: string;
  approximate: boolean;
  by_domain: SkillDomainCost[];
  skills: Skill[];
}

export interface SkillHealthFinding {
  id: number;
  skill_name?: string;
  check_type: string;
  severity: string;
  message: string;
  detail?: string;
  detected_at: string;
}

export interface SkillHealthReport {
  findings: SkillHealthFinding[];
  by_severity: Record<string, number>;
  by_check_type: Record<string, number>;
  total_skills: number;
  healthy_skills: number;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, authHeaders({ signal }));
  if (!res.ok) {
    throw new ApiError(res.status, await responseErrorMessage(res));
  }
  return (await res.json()) as T;
}

export async function fetchSkills(signal?: AbortSignal): Promise<Skill[]> {
  const body = await getJSON<{ skills: Skill[] }>("/skills", signal);
  return body.skills ?? [];
}

export function fetchSkillCost(
  signal?: AbortSignal,
): Promise<SkillTokenCostReport> {
  return getJSON<SkillTokenCostReport>("/skills/cost", signal);
}

export function fetchSkillHealth(
  signal?: AbortSignal,
): Promise<SkillHealthReport> {
  return getJSON<SkillHealthReport>("/skills/health", signal);
}
