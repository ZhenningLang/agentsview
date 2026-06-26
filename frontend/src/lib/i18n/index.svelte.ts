// Lightweight runes-based i18n. Default locale = zh, persisted to localStorage.
// t() reads the reactive locale, so calling it inside a Svelte template makes
// that template re-render on locale change. en is the fallback dictionary.
import { zh } from "./messages/zh";
import { en } from "./messages/en";

export type Locale = "zh" | "en";

const dicts: Record<Locale, Record<string, string>> = { zh, en };
const STORAGE_KEY = "agentsview.locale";

function readStoredLocale(): Locale {
  try {
    const saved = globalThis.localStorage?.getItem(STORAGE_KEY);
    if (saved === "zh" || saved === "en") return saved;
  } catch {
    // localStorage unavailable (SSR / tests without jsdom) → fall through
  }
  return "zh";
}

// Reactive locale holder. Exported as state so components can read i18n.locale.
export const i18n = $state<{ locale: Locale }>({ locale: readStoredLocale() });

export function setLocale(locale: Locale): void {
  i18n.locale = locale;
  try {
    globalThis.localStorage?.setItem(STORAGE_KEY, locale);
  } catch {
    // best-effort persistence
  }
}

export const LOCALES: Locale[] = ["zh", "en"];

// Translate a key with optional {param} interpolation. Missing keys fall back
// to en, then to the raw key (so a missing translation is visible, not blank).
export function t(key: string, params?: Record<string, string | number>): string {
  const dict = dicts[i18n.locale] ?? dicts.zh;
  let str = dict[key] ?? en[key] ?? key;
  if (params) {
    for (const [name, value] of Object.entries(params)) {
      str = str.split(`{${name}}`).join(String(value));
    }
  }
  return str;
}
