// Hiragana/katakana, CJK ideographs (including extension A and compatibility
// forms), and hangul syllables. Two characters of these scripts usually carry
// as much meaning as three latin ones, so they search at a lower threshold.
const CJK_PATTERN = /[぀-ヿ㐀-䶿一-鿿豈-﫿가-힯]/;

export const MIN_QUERY_LENGTH_LATIN = 3;
export const MIN_QUERY_LENGTH_CJK = 2;

export function minQueryLength(query: string): number {
  return CJK_PATTERN.test(query) ? MIN_QUERY_LENGTH_CJK : MIN_QUERY_LENGTH_LATIN;
}

export function isSearchableQuery(query: string): boolean {
  return query.length >= minQueryLength(query);
}
