# Gate 1: FAIL

Date: 2026-10-17. Inputs: E1 (skipped, no bill), E2, E3a pilot, E4.

**Question.** Does a cross-provider route or delegation beat the solo baseline on cost per
accepted task, at an accept rate that is not inferior?

**Answer: no.** Delegation matched solo on quality but not on cost. Config routing changed nothing.
Neither clears the gate (PASS needs the cost-ratio upper bound below 1.0; PROVISIONAL needs it
below 0.9). Per the plan: skip S4 and S5, ship the ledger alone, and do S6.

## E3a pilot: 31 tasks, 3 arms, paired

| Arm | Accepted | Accept rate | Total $ (API-equivalent) | $ per accepted | Median minutes |
| --- | --- | --- | --- | --- | --- |
| A solo, claude-opus-5-5 | 27 | 87% | 23.09 | 0.86 | 2.0 |
| B routed by harness config | 27 | 87% | 23.87 | 0.88 | 1.9 |
| C delegated to cc-delegate workers | 28 | 90% | 24.14 | 0.86 | 3.9 |

Paired bootstrap against A (10,000 resamples, [results](experiments/E3/results/pilot.csv)):

| Arm | $/accepted ratio | 95% CI | One-sided 95% upper bound | Accept-rate diff (one-sided 95% lower bound) | Quality at a 10-point margin |
| --- | --- | --- | --- | --- | --- |
| B | 1.03 | [0.98, 1.09] | 1.08 | +0.00 (+0.00) | non-inferior |
| C | 1.01 | [0.84, 1.17] | 1.15 | +0.03 (-0.06) | non-inferior |

Sensitivity: without T32 and T33 (unsolvable in every arm; see threats), C is 1.03 [0.85, 1.19],
upper bound 1.17. Same verdict.

## Why delegation did not save money

- Arm C's cost is 93% the Opus orchestrator ($22.49) and 7% the workers ($1.65). Planning, reading
  the code and reviewing diffs cost Opus 97% of what doing the change itself cost.
- Overhead dominates small tasks. By the solo cost of each task, in thirds:

| Task size (solo cost) | C / A cost | A accepted | C accepted |
| --- | --- | --- | --- |
| small ($0.17-$0.51) | 1.24 | 10/10 | 10/10 |
| medium ($0.52-$0.89) | 1.06 | 9/10 | 9/10 |
| large ($0.91-$1.62) | 0.99 | 8/11 | 9/11 |

  The trend points the right way for larger work, but this suite has no task large enough to show
  a win, and the largest third alone is not a powered test.

- Arm B is not a real treatment: 0 of 31 runs used the configured subagent model. Opus spawned no
  subagents on tasks this size, so routing subagents and background work by config moved no money.
  This matches E2 and S1: subagent and background traffic is a small share of spend.

## Power

The pilot already shows non-inferiority for C at the 10-point margin (lower bound -0.06, discordance
0.10; the formula asks for about 34 paired tasks, and 31 with this effect clear it). E3b, the powered
set, is not run: the gauntlet allows it past the pilot only when the pilot's cost-ratio upper bound
is below 0.9, and here it is 1.15. More tasks would narrow the interval around a ratio of 1.0; they
would not turn it into a win.

## What did save money (from E2 and E4)

- Choosing the main-loop model, not delegating: moving solo frontier calls to the cheapest frontier
  model is 0.76x at list price with no tier drop; to a strong-tier model 0.39x-0.41x (quality untested).
- Flat plans: 25-31x list-price value on $20 plans at this volume (Claude and ChatGPT), and $10.73 of
  per-token spend that plans already covered.

## Threats to validity

- Tasks come from two repos the user owns; intervals are per task, not clustered by repo.
- Tasks are small (median 2 minutes solo). Delegation may behave differently on multi-hour work.
- Tests were visible as the spec (D6); graded against the commit's full test tree plus a regression set.
- T32 and T33 check exact catalog prices that the commit message does not state; no arm could pass.
- Cost is the official client's API-equivalent figure under a plan, plus worker tokens at catalog
  prices; it is not a bill (E1 skipped).
- One model family as orchestrator (Opus 5.5) and one worker (DeepSeek V4.1 Flash).
