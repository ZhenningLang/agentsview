import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));

test("frontend test script runs API generator node tests", () => {
  assert.match(
    pkg.scripts.test,
    /node --test scripts\/\*\.node-test\.mjs/,
  );
  assert.match(pkg.scripts.test, /vitest run/);
});
