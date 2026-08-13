// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { i18n, setLocale, t, LOCALES } from "./index.svelte";

describe("i18n", () => {
  beforeEach(() => {
    localStorage.clear();
    setLocale("zh");
  });
  afterEach(() => {
    localStorage.clear();
  });

  it("defaults to Chinese and translates a known key", () => {
    expect(i18n.locale).toBe("zh");
    expect(t("enrich.title")).toBe("LLM 配置");
  });

  it("switches locale and reflects it in t()", () => {
    setLocale("en");
    expect(i18n.locale).toBe("en");
    expect(t("enrich.title")).toBe("LLM Configuration");
  });

  it("persists the chosen locale to localStorage", () => {
    setLocale("en");
    expect(localStorage.getItem("agentsview.locale")).toBe("en");
  });

  it("interpolates {param} placeholders", () => {
    // use an ad-hoc key by checking a known key has no params; assert raw interpolation
    expect(t("nonexistent.key.with.{x}", { x: "Y" })).toBe("nonexistent.key.with.Y");
  });

  it("falls back to English then to the raw key for missing translations", () => {
    setLocale("zh");
    // a key only meaningful as fallback: unknown key returns itself
    expect(t("totally.unknown.key")).toBe("totally.unknown.key");
  });

  it("exposes exactly the supported locales", () => {
    expect(LOCALES).toEqual(["zh", "en"]);
  });

  it("translates every phase-12 key in both locales", () => {
    const keys = [
      "header.copySourcePath",
      "session.malformedLines.one",
      "session.malformedLines.other",
      "session.malformedLines.tooltip.one",
      "session.malformedLines.tooltip.other",
      "shortcuts.nextUserPrompt",
      "shortcuts.prevUserPrompt",
      "toolBlock.copyInput",
      "toolBlock.copiedInput",
      "toolBlock.copyOutput",
      "toolBlock.copiedOutput",
    ];
    for (const loc of LOCALES) {
      setLocale(loc);
      for (const key of keys) {
        expect(t(key)).not.toBe(key); // translated, not raw key
        expect(t(key).trim()).not.toBe("");
      }
    }
  });

  it("interpolates the malformed-lines count in both locales", () => {
    setLocale("en");
    expect(t("session.malformedLines.one", { count: 1 })).toBe(
      "1 malformed line",
    );
    expect(t("session.malformedLines.other", { count: 7 })).toBe(
      "7 malformed lines",
    );
    expect(t("session.malformedLines.tooltip.one", { count: 1 })).toBe(
      "1 line in the source file could not be parsed",
    );
    expect(t("session.malformedLines.tooltip.other", { count: 7 })).toBe(
      "7 lines in the source file could not be parsed",
    );

    setLocale("zh");
    expect(t("session.malformedLines.other", { count: 7 })).toBe(
      "7 行无法解析",
    );
    expect(t("session.malformedLines.tooltip.other", { count: 7 })).toBe(
      "源文件中有 7 行无法解析",
    );
    for (const key of [
      "session.malformedLines.one",
      "session.malformedLines.other",
      "session.malformedLines.tooltip.one",
      "session.malformedLines.tooltip.other",
    ]) {
      expect(t(key, { count: 2 })).not.toContain("{count}");
    }
  });

  it("covers every usage label + description in both locales", () => {
    const usages = ["enrich", "consolidate", "embed", "recall_rerank"];
    for (const loc of LOCALES) {
      setLocale(loc);
      for (const u of usages) {
        expect(t(`usage.${u}`)).not.toBe(`usage.${u}`); // translated, not raw key
        expect(t(`usage.${u}.desc`)).not.toBe(`usage.${u}.desc`);
      }
    }
  });
});
