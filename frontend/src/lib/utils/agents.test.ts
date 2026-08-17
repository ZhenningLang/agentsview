import { describe, it, expect } from "vitest";
import {
  KNOWN_AGENTS,
  accentForeground,
  agentColor,
  agentForeground,
  agentLabel,
} from "./agents.js";

describe("KNOWN_AGENTS", () => {
  it("contains all expected agents", () => {
    const names = KNOWN_AGENTS.map((a) => a.name);
    expect(names).toEqual([
      "claude",
      "codex",
      "copilot",
      "gemini",
      "opencode",
      "openhands",
      "cursor",
      "amp",
      "zencoder",
      "zed",
      "vscode-copilot",
      "pi",
      "qwen",
      "openclaw",
      "qclaw",
      "iflow",
      "kimi",
      "kimicode",
      "claude-ai",
      "chatgpt",
      "kiro",
      "kiro-ide",
      "cortex",
      "workbuddy",
      "piebald",
      "antigravity",
      "antigravity-cli",
    ]);
  });

  it("has a color for every agent", () => {
    for (const agent of KNOWN_AGENTS) {
      expect(agent.color).toMatch(/^var\(--accent-/);
    }
  });
});

describe("agentColor", () => {
  it("returns correct color for known agents", () => {
    expect(agentColor("claude")).toBe(
      "var(--accent-blue)",
    );
    expect(agentColor("codex")).toBe(
      "var(--accent-green)",
    );
    expect(agentColor("copilot")).toBe(
      "var(--accent-amber)",
    );
    expect(agentColor("gemini")).toBe(
      "var(--accent-rose)",
    );
    expect(agentColor("opencode")).toBe(
      "var(--accent-purple)",
    );
    expect(agentColor("openhands")).toBe(
      "var(--accent-teal)",
    );
    expect(agentColor("cursor")).toBe(
      "var(--accent-black)",
    );
    expect(agentColor("amp")).toBe(
      "var(--accent-coral)",
    );
    expect(agentColor("zencoder")).toBe(
      "var(--accent-red)",
    );
    expect(agentColor("zed")).toBe(
      "var(--accent-green)",
    );
    expect(agentColor("pi")).toBe(
      "var(--accent-indigo)",
    );
    expect(agentColor("qwen")).toBe(
      "var(--accent-cyan)",
    );
    expect(agentColor("vscode-copilot")).toBe(
      "var(--accent-teal)",
    );
    expect(agentColor("qclaw")).toBe(
      "var(--accent-orange)",
    );
    expect(agentColor("workbuddy")).toBe(
      "var(--accent-violet)",
    );
    expect(agentColor("piebald")).toBe(
      "var(--accent-orange)",
    );
  });

  it("falls back to blue for unknown agents", () => {
    expect(agentColor("unknown")).toBe(
      "var(--accent-blue)",
    );
    expect(agentColor("")).toBe("var(--accent-blue)");
  });
});

describe("agentLabel", () => {
  it("returns explicit labels for hyphenated agents", () => {
    expect(agentLabel("vscode-copilot")).toBe(
      "VS Code Copilot",
    );
    expect(agentLabel("openhands")).toBe("OpenHands");
    expect(agentLabel("openclaw")).toBe("OpenClaw");
    expect(agentLabel("qclaw")).toBe("QClaw");
    expect(agentLabel("iflow")).toBe("iFlow");
    expect(agentLabel("workbuddy")).toBe("WorkBuddy");
    expect(agentLabel("piebald")).toBe("Piebald");
    expect(agentLabel("zed")).toBe("Zed");
    expect(agentLabel("qwen")).toBe("Qwen Code");
  });

  it("capitalizes simple agent names", () => {
    expect(agentLabel("claude")).toBe("Claude");
    expect(agentLabel("gemini")).toBe("Gemini");
  });
});

// Phase 19 (e65fe7a3): every accent fill that carries text needs a paired
// readable foreground token; hard-coded white is not acceptable.
describe("Phase 19 agentForeground", () => {
  it("returns the paired foreground token for every known agent fill", () => {
    for (const agent of KNOWN_AGENTS) {
      const color = agentColor(agent.name);
      const token = color.match(/^var\(--accent-([a-z]+)\)$/)?.[1];
      expect(token, `${agent.name} fill must be an accent token`).toBeTruthy();
      expect(agentForeground(agent.name)).toBe(
        `var(--accent-${token}-foreground)`,
      );
    }
  });

  it("covers more than one foreground token across the roster", () => {
    const distinct = new Set(
      KNOWN_AGENTS.map((agent) => agentForeground(agent.name)),
    );
    expect(distinct.size).toBeGreaterThan(1);
  });

  it("falls back to the blue pair for unknown agents", () => {
    expect(agentForeground("unknown")).toBe(
      "var(--accent-blue-foreground)",
    );
    expect(agentForeground("")).toBe("var(--accent-blue-foreground)");
  });

  it("uses non-blue foregrounds for non-blue agent fills", () => {
    expect(agentForeground("codex")).toBe("var(--accent-green-foreground)");
    expect(agentForeground("opencode")).toBe(
      "var(--accent-purple-foreground)",
    );
    expect(agentForeground("claude-ai")).toBe(
      "var(--accent-violet-foreground)",
    );
    expect(agentForeground("kimicode")).toBe(
      "var(--accent-indigo-foreground)",
    );
  });
});

describe("Phase 19 accentForeground", () => {
  it("maps every accent fill token to its foreground token", () => {
    const fills = [
      "blue",
      "rose",
      "purple",
      "amber",
      "green",
      "coral",
      "black",
      "teal",
      "red",
      "indigo",
      "orange",
      "sky",
      "pink",
      "lime",
      "cyan",
      "violet",
    ];
    for (const fill of fills) {
      expect(accentForeground(`var(--accent-${fill})`)).toBe(
        `var(--accent-${fill}-foreground)`,
      );
    }
  });

  it("falls back to the blue foreground for unmapped fills", () => {
    expect(accentForeground("#ff0000")).toBe(
      "var(--accent-blue-foreground)",
    );
    expect(accentForeground("var(--cat-read)")).toBe(
      "var(--accent-blue-foreground)",
    );
  });
});
