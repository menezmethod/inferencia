import { test } from "node:test";
import assert from "node:assert/strict";

// decide.mjs imports Laya at the top; the pure functions are tested without loading the model.
const { startTier, state, QUESTIONS } = await import("./decide.mjs");

test("fast when p_fast reaches the threshold", () => {
  assert.equal(startTier({ fast_ok: { noul: 0.5 }, tier: { choice: "frontier" } }), "fast");
});

test("never fast below the threshold", () => {
  assert.equal(startTier({ fast_ok: { noul: 0.2 }, tier: { choice: "fast" } }), "strong");
  assert.equal(startTier({ fast_ok: { noul: 0.2 }, tier: { choice: "frontier" } }), "frontier");
});

test("state carries only the prompt and visible-spec sizes", () => {
  const s = state({ task: "T1", prompt: "p", packages: 1, test_files: 2, test_lines: 3, extra: "x" });
  assert.deepEqual(s, { prompt: "p", visible_tests: { packages: 1, test_files: 2, test_lines: 3 } });
});

test("questions are frozen", () => {
  assert.deepEqual(Object.keys(QUESTIONS), ["tier", "difficulty", "fast_ok"]);
});
