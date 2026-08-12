import { describe, it, expect } from "vitest";
import {
  isSearchableQuery,
  minQueryLength,
  MIN_QUERY_LENGTH_CJK,
  MIN_QUERY_LENGTH_LATIN,
} from "./search-query.js";

describe("minQueryLength", () => {
  const cases: Array<{ name: string; query: string; want: number }> = [
    { name: "empty", query: "", want: MIN_QUERY_LENGTH_LATIN },
    { name: "latin", query: "api", want: MIN_QUERY_LENGTH_LATIN },
    { name: "digits", query: "42", want: MIN_QUERY_LENGTH_LATIN },
    { name: "simplified chinese", query: "侯爽", want: MIN_QUERY_LENGTH_CJK },
    { name: "traditional chinese", query: "設計", want: MIN_QUERY_LENGTH_CJK },
    { name: "hiragana", query: "ひら", want: MIN_QUERY_LENGTH_CJK },
    { name: "katakana", query: "カナ", want: MIN_QUERY_LENGTH_CJK },
    { name: "hangul", query: "한글", want: MIN_QUERY_LENGTH_CJK },
    { name: "mixed cjk and latin", query: "侯s", want: MIN_QUERY_LENGTH_CJK },
    { name: "cjk punctuation only", query: "，。", want: MIN_QUERY_LENGTH_LATIN },
  ];

  for (const c of cases) {
    it(`returns ${c.want} for ${c.name}`, () => {
      expect(minQueryLength(c.query)).toBe(c.want);
    });
  }
});

describe("isSearchableQuery", () => {
  const cases: Array<{ name: string; query: string; want: boolean }> = [
    { name: "empty", query: "", want: false },
    { name: "one latin char", query: "a", want: false },
    { name: "two latin chars", query: "ab", want: false },
    { name: "three latin chars", query: "abc", want: true },
    { name: "one cjk char", query: "侯", want: false },
    { name: "two cjk chars", query: "侯爽", want: true },
    { name: "three cjk chars", query: "侯爽好", want: true },
    { name: "cjk plus latin", query: "侯s", want: true },
    { name: "latin plus space plus latin", query: "侯 s", want: true },
  ];

  for (const c of cases) {
    it(`${c.want ? "accepts" : "rejects"} ${c.name}`, () => {
      expect(isSearchableQuery(c.query)).toBe(c.want);
    });
  }
});
