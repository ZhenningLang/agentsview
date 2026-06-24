import type { Session } from "../api/types.js";
import { previewMessage } from "./messages.js";

export interface SessionTitleInput {
  project: string;
  first_message?: string | null;
  display_name?: string | null;
  llm_title?: string | null;
}

export interface SessionTitleResult {
  text: string;
  isShell: boolean;
  source: "display_name" | "llm_title" | "first_message" | "project";
}

export function originalSessionTitle(
  session: SessionTitleInput,
): SessionTitleResult {
  const name = session.display_name ?? null;
  if (name) {
    return { text: name, isShell: false, source: "display_name" };
  }

  let msg = session.first_message ?? "";
  if (msg.includes("<teammate-message")) {
    msg = msg
      .replace(/<teammate-message[^>]*>/g, "")
      .replace(/<\/teammate-message>/g, "")
      .trim();
    const taskMatch = msg.match(/Task\s*#?\d+[:\s]+(.+?)(?:\s+\d+\.|$)/s);
    if (taskMatch) {
      return {
        text: taskMatch[1]!.trim(),
        isShell: false,
        source: "first_message",
      };
    }
    const afterTeam = msg.match(/team[."]\s*[^.]*?[.]\s+(.+)/s)
      ?? msg.match(/You are a teammate[^.]*\.\s+(.+)/s);
    if (afterTeam) {
      return {
        text: afterTeam[1]!.trim(),
        isShell: false,
        source: "first_message",
      };
    }
  }

  const preview = previewMessage(msg);
  if (preview.text) {
    return {
      text: preview.text,
      isShell: preview.isShell,
      source: "first_message",
    };
  }
  return { text: session.project, isShell: false, source: "project" };
}

export function sessionTitle(
  session: SessionTitleInput | Session,
  useLlmTitle: boolean,
): SessionTitleResult {
  const llmTitle = session.llm_title?.trim();
  if (useLlmTitle && llmTitle) {
    return { text: llmTitle, isShell: false, source: "llm_title" };
  }
  return originalSessionTitle(session);
}
