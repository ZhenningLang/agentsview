<script lang="ts">
  import { onMount } from "svelte";
  import SettingsSection from "./SettingsSection.svelte";
  import {
    fetchMemoryBackupStatus,
    connectMemoryBackup,
    fetchBackupPushStatus,
    setBackupPushEnabled,
    type BackupPushStatus,
  } from "../../api/memoryBackup";

  let repoInput: string = $state("");
  let linkedRepo: string = $state("");
  let linked: boolean = $state(false);
  let connecting: boolean = $state(false);
  let error: string | null = $state(null);
  let success: string | null = $state(null);

  // Background backup-push state.
  let push: BackupPushStatus | null = $state(null);
  let togglingPush: boolean = $state(false);
  let pushError: string | null = $state(null);

  async function loadPushStatus() {
    try {
      push = await fetchBackupPushStatus();
    } catch {
      // Fail-soft: leave the push status hidden if it is unreachable.
      push = null;
    }
  }

  onMount(async () => {
    try {
      const status = await fetchMemoryBackupStatus();
      linkedRepo = status.repo;
      linked = status.linked;
    } catch {
      // Fail-open: an unreachable status endpoint just leaves the form blank.
    }
    await loadPushStatus();
  });

  async function handleTogglePush() {
    if (!push) return;
    togglingPush = true;
    pushError = null;
    try {
      const res = await setBackupPushEnabled(!push.enabled);
      // Reload to pick up the immediate-cycle result (last success / error).
      push = { ...push, enabled: res.enabled, available: res.available };
      await loadPushStatus();
    } catch (e) {
      pushError = e instanceof Error ? e.message : "Failed to toggle backup push";
    } finally {
      togglingPush = false;
    }
  }

  async function handleConnect() {
    const target = repoInput.trim();
    if (!target) return;
    connecting = true;
    error = null;
    success = null;
    try {
      // The server reads no clock for the marker; embed the time client-side.
      const markerContent = `agentsview memory backup marker\nclaimed_at: ${new Date().toISOString()}\n`;
      const result = await connectMemoryBackup(target, markerContent);
      linkedRepo = result.repo;
      linked = result.linked;
      repoInput = "";
      success =
        result.outcome === "created"
          ? `Created and linked private repo ${result.repo}.`
          : `Linked private repo ${result.repo}.`;
    } catch (e) {
      error = e instanceof Error ? e.message : "Failed to connect backup repo";
    } finally {
      connecting = false;
    }
  }
</script>

<SettingsSection
  title="Memory Backup"
  description="Connect or create a PRIVATE GitHub repo (via the local gh CLI) to back up memory. Memory never goes to a public repo."
>
  <div class="status-row">
    <span class="status-label">Status</span>
    <span class="status-value" class:configured={linked}>
      {linked && linkedRepo ? `Connected: ${linkedRepo}` : "Not connected"}
    </span>
  </div>

  <div class="repo-row">
    <input
      class="setting-input"
      type="text"
      placeholder="namespace or owner/repo or repo URL"
      bind:value={repoInput}
      onkeydown={(e) => {
        if (e.key === "Enter") handleConnect();
      }}
    />
    <button
      class="save-btn"
      disabled={connecting || !repoInput.trim()}
      onclick={handleConnect}
    >
      {connecting ? "Connecting..." : "Connect"}
    </button>
  </div>

  <p class="hint">
    A bare namespace creates <code>&lt;owner&gt;/agent-memory</code>. The target
    must be private; an existing repo with foreign content is rejected.
  </p>

  {#if error}
    <p class="msg error">{error}</p>
  {/if}
  {#if success}
    <p class="msg success">{success}</p>
  {/if}

  {#if push}
    <div class="push-block">
      <div class="push-header">
        <span class="status-label">Auto push</span>
        <button
          class="toggle-btn"
          class:on={push.enabled}
          disabled={togglingPush || !push.available}
          onclick={handleTogglePush}
          title={!push.available
            ? "Connect a private repo first (and run a writable, local store)"
            : ""}
        >
          {#if togglingPush}
            ...
          {:else}
            {push.enabled ? "On" : "Off"}
          {/if}
        </button>
      </div>

      {#if push.last_success_at}
        <p class="push-line ok">
          Last push succeeded at {push.last_success_at}.
        </p>
      {/if}
      {#if push.last_error}
        <p class="push-line bad">
          Last push failed{push.last_error_at
            ? ` at ${push.last_error_at}`
            : ""}: {push.last_error}
        </p>
      {/if}
      {#if !push.last_success_at && !push.last_error}
        <p class="push-line muted">No backup pushed yet.</p>
      {/if}
      {#if pushError}
        <p class="msg error">{pushError}</p>
      {/if}

      <p class="hint">
        The timer is bound to the agentsview process (no system cron): backups
        run only while agentsview is running. Memory never goes to a public repo.
      </p>
    </div>
  {/if}
</SettingsSection>

<style>
  .status-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .status-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .status-value {
    font-size: 12px;
    color: var(--text-muted);
  }

  .status-value.configured {
    color: var(--accent-green);
  }

  .repo-row {
    display: flex;
    gap: 8px;
  }

  .setting-input {
    flex: 1;
    height: 30px;
    padding: 0 10px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-family: var(--font-mono, monospace);
    color: var(--text-primary);
    background: var(--bg-inset);
    border: 1px solid var(--border-muted);
    transition: border-color 0.15s;
  }

  .setting-input:focus {
    outline: none;
    border-color: var(--accent-blue);
  }

  .save-btn {
    height: 30px;
    padding: 0 14px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    color: white;
    background: var(--accent-blue);
    border: none;
    cursor: pointer;
    white-space: nowrap;
    transition: opacity 0.12s;
  }

  .save-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .save-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .hint {
    font-size: 11px;
    color: var(--text-muted);
    margin: 0;
  }

  .hint code {
    font-family: var(--font-mono, monospace);
  }

  .msg {
    font-size: 11px;
    margin: 0;
  }

  .msg.error {
    color: var(--accent-red, #ef4444);
  }

  .msg.success {
    color: var(--accent-green, #22c55e);
  }

  .push-block {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border-muted);
  }

  .push-header {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .toggle-btn {
    height: 26px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    color: var(--text-primary);
    background: var(--bg-inset);
    border: 1px solid var(--border-muted);
    cursor: pointer;
    transition:
      background 0.12s,
      color 0.12s;
  }

  .toggle-btn.on {
    color: white;
    background: var(--accent-green, #22c55e);
    border-color: transparent;
  }

  .toggle-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .push-line {
    font-size: 11px;
    margin: 0;
  }

  .push-line.ok {
    color: var(--accent-green, #22c55e);
  }

  .push-line.bad {
    color: var(--accent-red, #ef4444);
  }

  .push-line.muted {
    color: var(--text-muted);
  }
</style>
