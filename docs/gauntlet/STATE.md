# Gauntlet state

Branch: `gauntlet/v4`. Budget: $25.00. Spent: $0.00 (see [spend.csv](spend.csv)).
Statuses: TODO, IN-PROGRESS, DONE, BLOCKED, SKIPPED-BUDGET, SKIPPED-NO-BILL, SKIPPED-GATE.

## S0 Setup

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S0-1 | State file, spend ledger, decisions log | DONE | [S0](evidence/S0.md) |
| S0-2 | Baseline: make vet, make test (-race) | DONE | [S0](evidence/S0.md) |
| S0-3 | OpenCode Go billing model | DONE | [S0](evidence/S0.md) |

## S1 Measure (P-1)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S1-1 | Usage record + Claude Code adapter | DONE | [S1](evidence/S1.md) |
| S1-2 | Codex CLI adapter | DONE | [S1](evidence/S1.md) |
| S1-3 | OpenCode adapter (sqlite3, read-only) | DONE | [S1](evidence/S1.md) |
| S1-4 | Command Code adapter | DONE | [S1](evidence/S1.md) |
| S1-5 | `inferencia measure` command + report | DONE | [S1](evidence/S1.md) |
| S1-6 | Golden numbers check (4 sources) | DONE | [S1](evidence/S1.md) |

## S2 Catalog and savings (P0)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S2-1 | catalog.yaml with sourced prices | DONE | [S2](evidence/S2.md) |
| S2-2 | One price function, fail loud on unknown model | DONE | [S2](evidence/S2.md) |
| S2-3 | `inferencia savings` (counterfactuals with range) | DONE | [S2](evidence/S2.md) |
| S2-4 | `inferencia subs` (API-equivalent value per plan) | DONE | [S2](evidence/S2.md) |

## S3 Experiments (Gate 1 inputs)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| E1 | Ledger vs a billed source | SKIPPED-NO-BILL (no API key in the shell) | |
| E2 | Offline replay under 5 policies | DONE | [E2](experiments/E2/README.md), [review](evidence/E2-review.md) |
| E3-suite | Task suite (30+) | DONE: 31 valid of 34, hardened harness | [suite](experiments/E3/suite.tsv), [review](evidence/E3-harness-review.md) |
| E3a | Pilot: 31 tasks x 3 arms | DONE | [E3](experiments/E3/README.md), [results](experiments/E3/results/pilot.csv) |
| E3b | Powered set | SKIPPED-GATE (pilot cost upper bound 1.15, not below 0.9; quality already non-inferior) | [GATE1](GATE1.md) |
| E4 | Subscription value + dispatch simulation | DONE | [E4](experiments/E4/README.md) |
| GATE1 | Gate 1 decision (deadline 2026-11-11) | DONE: FAIL | [GATE1](GATE1.md) |

## S4 Gateway (P1; PASS, or PROVISIONAL in shadow mode only)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S4-1 | Provider interface replaces Backend | SKIPPED-GATE | |
| S4-2 | Anthropic Messages ingress | SKIPPED-GATE | |
| S4-3 | Egress: Anthropic native, OpenAI-compatible, byte-stable pass-through | SKIPPED-GATE | |
| S4-4 | Sticky session routing | SKIPPED-GATE | |
| S4-5 | Failover + circuit breaker + typed errors | SKIPPED-GATE | |
| S4-6 | Budgets, shadow mode, why, off/on, fail-open | SKIPPED-GATE | |
| S4-7 | OTel GenAI attributes + route reason | SKIPPED-GATE | |
| E5 | Gateway overhead | SKIPPED-GATE | |
| E6 | Fault injection | SKIPPED-GATE | |
| E7 | Cache preservation | SKIPPED-GATE | |
| E8 | Conformance fixtures | SKIPPED-GATE | |
| E9 | Security tests, govulncheck, SBOM | SKIPPED-GATE | |

## S5 Delegate engine (P2; Gate 1 PASS only)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S5-1 | SQLite step log, crash-resume test | SKIPPED-GATE | |
| S5-2 | Bundles, verify allowlist, sandboxed verify | SKIPPED-GATE | |
| S5-3 | Subscription-first dispatch | SKIPPED-GATE | |
| GATE2 | Gate 2 decision | SKIPPED-GATE | |

## S6 Education document

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S6 | docs/EDUCATION.md | IN-PROGRESS | |

## Log

- 2026-10-17: S0 done. Baseline green.
- 2026-10-17: S1 done. Golden numbers match for OpenCode, Codex, Command Code. Findings F1 (Claude retention), F2 (Haiku calls now appear).
- 2026-10-17: S2 done. Findings F3-F6 (plans worth 25-31x at list prices; price tables disagree).
- 2026-10-17: E1 skipped: no billed source reachable (no OpenRouter or provider key exported).
- 2026-10-17: E2 done. Main loop off frontier: 0.41x; delegated only: 0.90x; naive router: 547 switches, $13 cache loss. Two replay bugs found by tests and fixed (mixed populations; recorded switches charged twice).
- 2026-10-17: E2 review fixes: per-subagent conversations, 1-hour rewrites, alias ids, Codex records never switch. New result: naive router with an expensive target costs 1.16x as-run ($233 cache loss).
- 2026-10-17: E3 suite: 30 valid tasks. E4 done: user is on OpenCode Go $10 (Kimi cap hit at $15.05); $10.73 avoidable per-token spend found.
- 2026-10-17: F7: router round-robin bug (map order) fixed, 4473f88.
- 2026-10-17: E3 harness hardened after review (3 blockers, 8 majors fixed); live isolation probe passed; pilot started. D6 (tests visible), D7 (Claude plan quota signal).
- 2026-10-17: App restart wiped the session scratchpad; the first pilot run (11 of 31 tasks) and its results were lost. Interim read on 8 tasks before the loss: all arms 100% accepted; $/accepted vs A: B 1.16x, C 1.28x; B never used its subagent model. Pilot relaunched in /private/tmp/inferencia-e3 with results copied to experiments/E3/results/pilot.csv after every task.
- 2026-10-17: E3 pilot complete (31 tasks, 7-day plan use 21%). Gate 1 FAIL: delegation matched solo on quality (non-inferior) and cost (1.01x, upper bound 1.15); config routing had no effect. S4/S5 skipped. Doing S6.
