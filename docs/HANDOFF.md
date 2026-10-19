# Handoff: inferencia v4 (2026-10-19)

Start here: read [EDUCATION.md](EDUCATION.md) (about 10 minutes). It explains the project and every result.

## Where things are

- Branch `gauntlet/v4`: 26 commits ahead of `main`, local only, never pushed. On purpose: `main`
  auto-deploys to the Pi.
- Working tree clean, except an untracked `.commandcode/` that is not part of this work.
- Plan doc: [Inferencia v4 plan](https://claude.ai/code/artifact/a64e4f3e-d3c3-44c1-8395-074617297e52)
  (opens with the outcome); repo copy in [PLAN_V4.md](PLAN_V4.md).
- The gauntlet ([GAUNTLET.md](GAUNTLET.md)) is complete: every item in
  [gauntlet/STATE.md](gauntlet/STATE.md) is DONE or SKIPPED.

## What was built: the ledger

| Command | What it does |
| --- | --- |
| `inferencia measure` | Spend, cache share, solo vs delegated, from Claude Code, Codex, OpenCode and Command Code logs |
| `inferencia savings` | List-price cost as run vs the same tokens on other models |
| `inferencia subs` | What each flat plan was worth per month |
| `inferencia replay` | Recorded sessions re-priced under routing policies, cache loss included |

Code: `internal/ledger`, `internal/catalog` (sourced prices in `catalog.yaml`), `internal/replay`,
`cmd/inferencia/measure.go`. All tests pass with the race detector. A real router bug was fixed on
the way (commit 4473f88: equal-load round-robin was not round-robin).

## What was proven

1. Delegating to cheap workers does not save money on this kind of work: 1.01x cost per accepted
   task (95% CI 0.84-1.17) at the same quality, in a 31-task paired pilot. Gate 1 failed, so the
   gateway (S4) and delegate engine (S5) were not built. See [gauntlet/GATE1.md](gauntlet/GATE1.md).
2. The main-loop model is the lever: 0.76x at list price by moving to the cheapest frontier model.
   Switching models mid-session can cost more than no router (1.16x).
   See [gauntlet/experiments/E2](gauntlet/experiments/E2/README.md).
3. Flat plans are the deal: the $20 plans delivered 25-31x their price in list-price value.
   See [gauntlet/experiments/E4](gauntlet/experiments/E4/README.md).

## Key files

| File | What |
| --- | --- |
| [EDUCATION.md](EDUCATION.md) | The full explainer |
| [gauntlet/GATE1.md](gauntlet/GATE1.md) | Pilot numbers and the verdict |
| [gauntlet/STATE.md](gauntlet/STATE.md) | Every item, its status and evidence link |
| [gauntlet/DECISIONS.md](gauntlet/DECISIONS.md) | D1-D7, the decisions taken and why |
| [gauntlet/evidence/](gauntlet/evidence/) | Per-stage evidence and adversarial reviews |
| [gauntlet/experiments/E3/](gauntlet/experiments/E3/) | Sandboxed pilot harness, reusable as is |
| [gauntlet/spend.csv](gauntlet/spend.csv) | Paid spend: about $1.65 of $25 |

## Open items (your call)

1. Push or merge: nothing is pushed. To share it, push `gauntlet/v4` and open a PR; never push to
   `main` directly.
2. E1, ledger vs a real bill: skipped for lack of a key. Export `OPENROUTER_API_KEY` and rerun it.
3. Raise Claude Code's `cleanupPeriodDays`: it deletes old transcripts, so ledger history disappears.
4. Next experiment worth running: a live main-loop model swap with the E3 harness (gpt-6-astra vs
   claude-opus-5-5 vs a strong-tier model), to check whether the 0.76x saving holds at equal quality.

Loose end: the first pilot run (11 tasks) was lost to an app restart; the full rerun replaced it, and
the loss is logged in STATE.md.
