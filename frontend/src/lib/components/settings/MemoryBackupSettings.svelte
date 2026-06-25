<script lang="ts">
  import { onMount } from "svelte";
  import SettingsSection from "./SettingsSection.svelte";
  import {
    fetchMemoryBackupStatus,
    connectMemoryBackup,
  } from "../../api/memoryBackup";

  let repoInput: string = $state("");
  let linkedRepo: string = $state("");
  let linked: boolean = $state(false);
  let connecting: boolean = $state(false);
  let error: string | null = $state(null);
  let success: string | null = $state(null);

  onMount(async () => {
    try {
      const status = await fetchMemoryBackupStatus();
      linkedRepo = status.repo;
      linked = status.linked;
    } catch {
      // Fail-open: an unreachable status endpoint just leaves the form blank.
    }
  });

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
</style>
