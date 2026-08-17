<script lang="ts">
  import type { Granularity } from "../../stores/analytics.svelte.js";

  interface Props {
    value: Granularity;
    onChange: (value: Granularity) => void;
    disabled?: boolean;
  }

  let { value, onChange, disabled = false }: Props = $props();

  const options: { value: Granularity; label: string }[] = [
    { value: "day", label: "Day" },
    { value: "week", label: "Week" },
    { value: "month", label: "Month" },
  ];
</script>

<div class="granularity-picker" aria-label="Trend granularity">
  {#each options as option}
    <button
      type="button"
      class:active={value === option.value}
      disabled={disabled}
      aria-pressed={value === option.value}
      onclick={() => onChange(option.value)}
    >
      {option.label}
    </button>
  {/each}
</div>

<style>
  .granularity-picker {
    display: inline-flex;
    gap: 2px;
    padding: 2px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
  }

  button {
    padding: 2px 8px;
    border: 0;
    border-radius: calc(var(--radius-sm) - 2px);
    background: transparent;
    color: var(--text-muted);
    font-size: 10px;
    cursor: pointer;
  }

  button.active {
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  button:disabled {
    cursor: wait;
    opacity: 0.6;
  }
</style>
