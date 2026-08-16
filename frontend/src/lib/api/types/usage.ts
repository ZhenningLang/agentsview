/** Usage types — match Go structs in internal/server/usage.go
 *  and internal/db/usage.go */

export interface UsageTotals {
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalCost: number;
  hasCost?: boolean;
  unpricedModels?: string[];
}

export interface ModelBreakdown {
  modelName: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface ProjectBreakdown {
  project: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface AgentBreakdown {
  agent: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface MachineBreakdown {
  machine: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface DailyUsageEntry {
  date: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalCost: number;
  hasCost?: boolean;
  unpricedModels?: string[];
  modelsUsed: string[];
  modelBreakdowns?: ModelBreakdown[];
  projectBreakdowns?: ProjectBreakdown[];
  agentBreakdowns?: AgentBreakdown[];
  machineBreakdowns?: MachineBreakdown[];
}

export interface ProjectTotal {
  project: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface ModelTotal {
  model: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface AgentTotal {
  agent: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface MachineTotal {
  machine: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  cost: number;
}

export interface CacheStats {
  cacheReadTokens: number;
  cacheCreationTokens: number;
  uncachedInputTokens: number;
  outputTokens: number;
  hitRate: number;
  savingsVsUncached: number;
}

export interface UsageSessionCounts {
  total: number;
  byProject: Record<string, number>;
  byAgent: Record<string, number>;
}

export interface UsageComparison {
  priorFrom: string;
  priorTo: string;
  priorTotalCost: number;
  deltaPct: number;
}

export interface UsageSummaryResponse {
  from: string;
  to: string;
  totals: UsageTotals;
  daily: DailyUsageEntry[];
  projectTotals: ProjectTotal[];
  modelTotals: ModelTotal[];
  agentTotals: AgentTotal[];
  machineTotals: MachineTotal[];
  sessionCounts: UsageSessionCounts;
  cacheStats: CacheStats;
  comparison?: UsageComparison;
}

export interface TopSessionEntry {
  sessionId: string;
  displayName: string;
  agent: string;
  project: string;
  startedAt: string;
  totalTokens: number;
  cost: number;
  hasCost?: boolean;
  unpricedModels?: string[];
}

export type TopUsageSessionsResponse = TopSessionEntry[];

export type PairwiseDimension = "model" | "project";

export interface PairwiseSide {
  dimension: PairwiseDimension;
  value: string;
  empty?: boolean;
}

export interface PairwiseUsageMetrics {
  totalCost: number;
  hasCost?: boolean;
  unpricedModels?: string[];
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalTokens: number;
  sessionCount: number;
  costPerSession?: number;
  tokensPerSession?: number;
}

export interface PairwiseDelta {
  totalCost: number;
  hasCost?: boolean;
  unpricedModels?: string[];
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalTokens: number;
  sessionCount: number;
  costPerSession?: number;
  tokensPerSession?: number;
  costRelativeChange?: number;
  tokensRelativeChange?: number;
}

export interface PairwiseComparisonResponse {
  left: PairwiseSide;
  right: PairwiseSide;
  leftMetrics: PairwiseUsageMetrics;
  rightMetrics: PairwiseUsageMetrics;
  deltas: PairwiseDelta;
}

export interface UsageParams {
  from?: string;
  to?: string;
  project?: string;
  machine?: string;
  agent?: string;
  model?: string;
  exclude_project?: string;
  exclude_agent?: string;
  exclude_model?: string;
  min_user_messages?: number;
  include_one_shot?: boolean;
  include_automated?: boolean;
  active_since?: string;
  timezone?: string;
}

export interface UsageTopSessionsParams extends UsageParams {
  limit?: number;
}
