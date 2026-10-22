# Routing evaluation

**Product.** Inferencia is a lightweight inference router. For each request, it automatically
selects the lowest-cost provider that can do the job well, and it needs almost no configuration.

**This document comes first.** It fixes the objective, the gates and the deciders before any
matrix data exists. The architecture (`docs/PLAN_V5.md`) is written after the results, and it
contains only the layers that the results support. Do not change a gate, a decider rule or a
Laya question after the first matrix run. If a change is necessary, record it in "Changes after
freeze" with the reason, and report the result with and without the change.

Frozen in: this commit (branch `eval/routing-matrix`). Old plans: [docs/archive/](../docs/archive/).

## 1. Objective

- **Accepted task:** after the run, the commit's whole test tree is restored, and
  `go build ./...`, the task's packages and the regression set pass. The model never sees the
  restored tests that it did not get as the spec. This is the hidden-test check.
- **Cost:** API-equivalent cost of every model call that a policy makes for a task, from the
  token counts in the run log, at the prices in `internal/catalog/catalog.yaml`. Cache reads and
  cache writes are priced separately. A failed attempt that a cascade escalates from is paid in
  full. A model with no catalog price makes the analysis fail. It is never priced at $0.
- **Metric:** cost per accepted task (CPA) = total cost of the policy over all tasks / number of
  accepted tasks.
- **Quality floor:** the accept rate must be non-inferior to always-Opus at a 10-point margin
  (one-sided 95% lower bound of the paired accept-rate difference > -0.10).

## 2. Raw material

| Source | What it gives | Where |
| --- | --- | --- |
| E3 suite | 31 tasks from real commits (17 inferencia, 14 private repo), validated: tests fail at the parent and pass at the commit 3 times | `docs/gauntlet/experiments/E3/suite.tsv` |
| E3 arm A rows | The Opus column (run 2026-10-17, same harness) | `docs/gauntlet/experiments/E3/results/pilot.csv` |
| Matrix runs | Per task and model: accepted, visible-test pass, tokens, cost, seconds, full stream log | `eval/matrix/` (the logs stay on the Mac) |
| Command Code dashboard | Requests and tokens per model for the billing month | Screenshot, 2026-10-22 (G4) |
| Laya | Source and API from the repository, not the blog | https://github.com/receptron/laya |

Jev is not evaluated: there is no TypeSafe API key. It enters with the same frozen questions
when a key exists.

## 3. Models (the matrix columns)

| Model | Tier | Provider | Tasks |
| --- | --- | --- | --- |
| claude-opus-5-5 | frontier | Claude plan | all 31 (E3 arm A rows, reused) |
| claude-sonnet-5-5 | strong | Claude plan | all 31 |
| claude-haiku-5-5 | fast | Claude plan | all 31 |
| deepseek-v4-1-flash | fast | Command Code via the local LiteLLM gateway | all 31 |
| muse-spark-1.3-contributor | fast | Command Code via the local LiteLLM gateway | 17 public inferencia tasks only (the provider may train on prompts) |

Every model runs in the same harness: Claude Code (`claude -p`) with the same prompt, tools,
sandbox, turn limit and time limit. Non-Claude models use `ANTHROPIC_BASE_URL` pointed at the
gateway. Thus the model is the only difference between columns.

## 4. Policies

Each policy is computed offline from the matrix. Nothing is rerun.

| Policy | Rule |
| --- | --- |
| always-X | One model for every task (one policy per model) |
| oracle | The cheapest model that was accepted on the task; Opus if none was accepted. A lower bound, not a real policy |
| random | Expected value of a uniform choice among the models that ran on the task |
| cascade-visible | Start at fast (deepseek), escalate to strong (sonnet), then frontier (opus) when the visible tests fail. This is the real failure signal that a router has |
| cascade-hidden | Same, but escalate on the hidden-test result. An optimistic upper bound |
| rules | Start tier from the frozen rules (section 5), then cascade-visible |
| laya | Start tier from Laya (section 5), then cascade-visible |

The tier ladder is fixed: fast = deepseek-v4-1-flash, strong = claude-sonnet-5-5,
frontier = claude-opus-5-5. Muse and Haiku are evaluated as always-X and in the oracle only.

## 5. Deciders (frozen)

A decider sees only what a router sees at the start of a session: the prompt, and the list and
size of the visible spec files. It does not see the answer diff or any matrix result.

**Rules** (`eval/deciders/rules.py`):

- frontier if the task touches 3 or more packages, or the visible tests have 800 or more lines;
- else strong if the task touches 2 packages, or the visible tests have 300 or more lines;
- else fast.

**Laya** (`eval/deciders/laya/decide.mjs`): one `systemOne` call per task, three questions:

- `tier` (choice): which model tier the change needs (fast / strong / frontier, with fixed
  descriptions);
- `difficulty` (score): 5-level rubric;
- `fast_ok` (noul): "A fast, low-cost coding model can implement this change so that all
  tests pass."

Start tier: fast if `fast_ok` >= 0.5, else the `tier` choice (never fast in that case). The 0.5
threshold is the primary result. A sweep of the threshold is a secondary result (APGR curve).

## 6. Gates (pass/fail, fixed now)

| Gate | Pass condition | If it fails |
| --- | --- | --- |
| **G1 Router value** (each decider and cascade-visible) | Quality floor holds, and the CPA ratio against the best fixed policy that also holds the quality floor has a one-sided 95% upper bound < 1.0. If no fixed policy cheaper than Opus holds the floor, the bound against always-Opus must be < 0.9 | The decider is not built. The router default is the best fixed policy |
| **G2 Calibration** (Laya `fast_ok`) | Brier score below the constant base-rate predictor, and ECE <= 0.10 (5 equal-width bins) against the deepseek column | Laya may pick tiers, but its probability is not used in an expected-cost formula |
| **G3 Overhead** (each decider) | p95 latency <= 1 s per decision, and decision cost <= 10% of the mean saving per task against always-Opus | The decider is not a default layer |
| **G4 Ledger truth** | `inferencia measure` for Command Code over the billing month is within 5% of the dashboard on requests and on tokens, or each gap has a named source | The ledger cannot be used for savings claims |
| **G5 Laya beats rules** | Laya passes G1, and its CPA ratio against rules has a 95% upper bound < 1.0 | Rules are the default decider; Laya stays optional |

The result is reported even if it is "a fixed model is enough" or "rules are enough".

Statistics: paired bootstrap over tasks, 10,000 resamples, seed 1. With 31 tasks, a ratio
interval narrower than about +/-0.15 is not expected; the gates use bounds, not point values.

## 7. Budget and guards

- Claude plan: the run stops when the 7-day window reaches 60% (the E3 guard, unchanged).
- Command Code: the run stops when its spend at catalog prices reaches $10 (`EVAL_MAX_CC_USD`).
  The billing month resets on 2026-10-30; on 2026-10-22, 66% of the monthly limit was used.
- The run checks the gateway before it starts and stops if the gateway is down.

## 8. Threats to validity

- 31 tasks from two repos, all Go, all small to medium (Opus cost $0.17-$1.62 per task).
- The Opus column is from 2026-10-17; the other columns are later. Model versions may differ.
- One harness. A non-Claude model may do better in its own native harness. This is acceptable:
  the router serves requests from this harness.
- The rules thresholds come from judgment, not from data. That is intentional: they are frozen
  before the data, so all 31 tasks are test data for them.
- No held-out repo yet. A third repo is the next step if a decider passes G1.

## 9. How to run

On the Mac. The steps and checks are in [matrix/RUN.md](matrix/RUN.md).

## Changes after freeze

None.
