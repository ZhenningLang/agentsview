<script lang="ts">
  import type { Message, Session } from "../../api/types.js";
  import {
    configureGeneratedClient,
    isRemoteConnection,
  } from "../../api/runtime.js";
  import {
    SessionsService,
    type ResumeRequest,
    type ResumeResponse,
  } from "../../api/generated/index";
  import { copyToClipboard } from "../../utils/clipboard.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { sync } from "../../stores/sync.svelte.js";
  import { CirclePlayIcon } from "../../icons.js";

  interface Props {
    message: Message;
    session?: Session | null;
    variant?: "header" | "tool";
  }

  let {
    message,
    session,
    variant = "header",
  }: Props = $props();

  let localFeedback = $state("");
  let feedbackTimer: ReturnType<typeof setTimeout>;

  let owningSession = $derived(
    session !== undefined
      ? session
      : sessions.sessions.find((s) => s.id === message.session_id) ??
        sessions.activeSession,
  );

  let canForkFromMessage = $derived(
    owningSession?.agent === "claude" &&
      !(owningSession?.id ?? "").includes("~") &&
      !(sync.readOnly && isRemoteConnection()),
  );

  function setFeedback(message: string) {
    clearTimeout(feedbackTimer);
    localFeedback = message;
    feedbackTimer = setTimeout(() => { localFeedback = ""; }, 2000);
  }

  async function handleForkFromHere() {
    if (!canForkFromMessage) return;
    clearTimeout(feedbackTimer);
    try {
      configureGeneratedClient();
      const resp =
        await SessionsService.postApiV1SessionsIdResume({
          id: message.session_id,
          requestBody: {
            ...(sync.readOnly && !isRemoteConnection()
              ? { command_only: true }
              : {}),
            from_ordinal: message.ordinal,
            fork_session: true,
          } satisfies ResumeRequest,
        }) as ResumeResponse;
      if (resp.launched) {
        setFeedback(`Resumed in ${resp.terminal ?? "terminal"}`);
        return;
      }
      if (resp.command) {
        const ok = await copyToClipboard(resp.command);
        setFeedback(ok ? "Command copied!" : "Failed");
        return;
      }
    } catch {
      // Fall through to visible feedback below.
    }
    setFeedback("Failed");
  }
</script>

{#if canForkFromMessage}
  <button
    type="button"
    class:fork-btn={variant === "header"}
    class:tool-fork-btn={variant === "tool"}
    title="Fork from this message"
    aria-label="Fork from this message"
    onclick={handleForkFromHere}
  >
    <CirclePlayIcon size="14" strokeWidth="1.8" aria-hidden="true" />
  </button>
{/if}

{#if localFeedback}
  <span class="fork-feedback" role="status">{localFeedback}</span>
{/if}

<style>
  button {
    border: none;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
  }

  .fork-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 4px;
    opacity: 0.75;
    transition: opacity 0.15s ease,
      background 0.15s ease,
      color 0.15s ease,
      transform 0.1s ease;
  }

  .fork-btn:hover,
  .fork-btn:focus-visible {
    opacity: 1;
  }

  @media (hover: none) {
    .fork-btn {
      opacity: 1;
    }
  }

  .fork-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-secondary);
  }

  .fork-btn:active {
    transform: scale(0.92);
  }

  .tool-fork-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 20px;
    border-radius: 4px;
    opacity: 0.7;
  }

  .tool-fork-btn:hover,
  .tool-fork-btn:focus-visible {
    background: var(--hover-bg);
    color: var(--accent-purple);
    opacity: 1;
  }

  .fork-feedback {
    font-size: 11px;
    color: var(--text-muted);
    animation: fade-in-out 1.5s ease-in-out;
  }

  @keyframes fade-in-out {
    0% { opacity: 0; }
    20% { opacity: 1; }
    80% { opacity: 1; }
    100% { opacity: 0; }
  }
</style>
