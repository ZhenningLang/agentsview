<script lang="ts">
  import { i18n, setLocale, t, type Locale } from "../../i18n/index.svelte";

  function onChange(event: Event) {
    setLocale((event.currentTarget as HTMLSelectElement).value as Locale);
  }
</script>

<label class="lang-switcher" aria-label={t("lang.label")}>
  <span class="sr-only">{t("lang.label")}</span>
  <select value={i18n.locale} onchange={onChange} data-testid="language-switcher">
    <option value="zh">{t("lang.zh")}</option>
    <option value="en">{t("lang.en")}</option>
  </select>
</label>

<style>
  .lang-switcher select {
    height: 30px;
    padding: 0 9px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    cursor: pointer;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
