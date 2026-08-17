export interface AgentMeta {
  name: string;
  color: string;
  label?: string;
}

export const KNOWN_AGENTS: readonly AgentMeta[] = [
  { name: "claude", color: "var(--accent-blue)" },
  { name: "codex", color: "var(--accent-green)" },
  { name: "copilot", color: "var(--accent-amber)" },
  { name: "gemini", color: "var(--accent-rose)" },
  { name: "opencode", color: "var(--accent-purple)" },
  { name: "openhands", color: "var(--accent-teal)", label: "OpenHands" },
  { name: "cursor", color: "var(--accent-black)" },
  { name: "amp", color: "var(--accent-coral)", label: "Amp" },
  { name: "zencoder", color: "var(--accent-red)", label: "Zencoder" },
  { name: "zed", color: "var(--accent-green)", label: "Zed" },
  {
    name: "vscode-copilot",
    color: "var(--accent-teal)",
    label: "VS Code Copilot",
  },
  { name: "pi", color: "var(--accent-indigo)", label: "Pi" },
  { name: "qwen", color: "var(--accent-cyan)", label: "Qwen Code" },
  {
    name: "openclaw",
    color: "var(--accent-orange)",
    label: "OpenClaw",
  },
  {
    name: "qclaw",
    color: "var(--accent-orange)",
    label: "QClaw",
  },
  { name: "iflow", color: "var(--accent-sky)", label: "iFlow" },
  { name: "kimi", color: "var(--accent-pink)", label: "Kimi" },
  {
    name: "kimicode",
    color: "var(--accent-indigo)",
    label: "Kimi Code",
  },
  { name: "claude-ai", color: "var(--accent-violet)", label: "Claude.ai" },
  { name: "chatgpt", color: "var(--accent-lime)", label: "ChatGPT" },
  { name: "kiro", color: "var(--accent-lime)", label: "Kiro" },
  { name: "kiro-ide", color: "var(--accent-lime)", label: "Kiro IDE" },
  { name: "cortex", color: "var(--accent-cyan)", label: "Cortex Code" },
  { name: "workbuddy", color: "var(--accent-violet)", label: "WorkBuddy" },
  { name: "piebald", color: "var(--accent-orange)", label: "Piebald" },
  {
    name: "antigravity",
    color: "var(--accent-violet)",
    label: "Antigravity",
  },
  {
    name: "antigravity-cli",
    color: "var(--accent-violet)",
    label: "Antigravity CLI",
  },
];

const agentColorMap = new Map(
  KNOWN_AGENTS.map((a) => [a.name, a.color]),
);

const defaultFillColor = "var(--accent-blue)";
const defaultForeground = "var(--accent-blue-foreground)";

/**
 * Readable foreground for each accent fill. The values are defined in
 * app.css for both themes; see component-source-guard.test.ts, which fails
 * if an accent fill is paired with a hard-coded white foreground instead.
 */
const accentForegroundMap = new Map([
  ["var(--accent-blue)", "var(--accent-blue-foreground)"],
  ["var(--accent-rose)", "var(--accent-rose-foreground)"],
  ["var(--accent-purple)", "var(--accent-purple-foreground)"],
  ["var(--accent-amber)", "var(--accent-amber-foreground)"],
  ["var(--accent-green)", "var(--accent-green-foreground)"],
  ["var(--accent-coral)", "var(--accent-coral-foreground)"],
  ["var(--accent-black)", "var(--accent-black-foreground)"],
  ["var(--accent-teal)", "var(--accent-teal-foreground)"],
  ["var(--accent-red)", "var(--accent-red-foreground)"],
  ["var(--accent-indigo)", "var(--accent-indigo-foreground)"],
  ["var(--accent-orange)", "var(--accent-orange-foreground)"],
  ["var(--accent-sky)", "var(--accent-sky-foreground)"],
  ["var(--accent-pink)", "var(--accent-pink-foreground)"],
  ["var(--accent-lime)", "var(--accent-lime-foreground)"],
  ["var(--accent-cyan)", "var(--accent-cyan-foreground)"],
  ["var(--accent-violet)", "var(--accent-violet-foreground)"],
]);

export function agentColor(agent: string): string {
  return agentColorMap.get(agent) ?? defaultFillColor;
}

/** Foreground token that stays readable on the given accent fill. */
export function accentForeground(color: string): string {
  return accentForegroundMap.get(color) ?? defaultForeground;
}

/** Foreground token for an agent's identity fill. */
export function agentForeground(agent: string): string {
  return accentForeground(agentColor(agent));
}

export function agentLabel(agent: string): string {
  const meta = KNOWN_AGENTS.find((a) => a.name === agent);
  if (meta?.label) return meta.label;
  return agent.charAt(0).toUpperCase() + agent.slice(1);
}
