// Frozen Laya decider (eval/README.md section 5). Do not change the questions or the
// threshold after the first matrix run.
//
// Usage: node decide.mjs < features.jsonl > decisions-laya.csv
// Runs locally (ONNX Runtime, CPU). The first run downloads about 1.7 GB of weights.
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { Laya } from "@receptron/laya";

const THRESHOLD = 0.5;

export const QUESTIONS = {
  tier: {
    type: "choice",
    instructions: "Which model tier does this coding change need to be implemented correctly?",
    criteria: {
      fast: "a small, low-cost model: simple, local change with clear tests",
      strong: "a mid-size model: changes across several functions or files, some design judgment",
      frontier: "the strongest model: complex logic, many packages, or subtle concurrency or protocol behavior",
    },
  },
  difficulty: {
    type: "score",
    instructions: "How difficult is this coding change?",
    criteria: ["trivial", "easy", "moderate", "hard", "very hard"],
  },
  fast_ok: {
    type: "noul",
    instructions: "A fast, low-cost coding model can implement this change so that all tests pass.",
  },
};

export function startTier(answers, threshold = THRESHOLD) {
  if (answers.fast_ok.noul >= threshold) return "fast";
  return answers.tier.choice === "fast" ? "strong" : answers.tier.choice;
}

export function state(f) {
  return { prompt: f.prompt, visible_tests: { packages: f.packages, test_files: f.test_files, test_lines: f.test_lines } };
}

async function main() {
  const laya = await Laya.load();
  console.log("decider,task,start_tier,p_fast,tier_choice,latency_ms,cost_usd");
  for await (const line of createInterface({ input: process.stdin })) {
    if (!line.trim()) continue;
    const f = JSON.parse(line);
    const t0 = performance.now();
    const r = await laya.systemOne(state(f), QUESTIONS);
    const ms = performance.now() - t0;
    const a = r.answers;
    // cost_usd is 0: Laya runs on the local CPU and calls no paid API (eval/README.md section 5).
    console.log(["laya", f.task, startTier(a), a.fast_ok.noul.toFixed(4), a.tier.choice, ms.toFixed(1), 0].join(","));
  }
  await laya.close();
}

if (process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1])) {
  await main();
}
