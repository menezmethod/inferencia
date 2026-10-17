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
| S1-1 | Usage record + Claude Code adapter | TODO | |
| S1-2 | Codex CLI adapter | TODO | |
| S1-3 | OpenCode adapter (sqlite3, read-only) | TODO | |
| S1-4 | Command Code adapter | TODO | |
| S1-5 | `inferencia measure` command + report | TODO | |
| S1-6 | Golden numbers check (4 sources) | TODO | |

## S2 Catalog and savings (P0)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S2-1 | catalog.yaml with sourced prices | TODO | |
| S2-2 | One price function, fail loud on unknown model | TODO | |
| S2-3 | `inferencia savings` (counterfactuals with range) | TODO | |
| S2-4 | `inferencia subs` (API-equivalent value per plan) | TODO | |

## S3 Experiments (Gate 1 inputs)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| E1 | Ledger vs a billed source | TODO | |
| E2 | Offline replay under 4 policies | TODO | |
| E3-suite | Task suite with hidden tests (30+) | TODO | |
| E3a | Pilot: 30 tasks x 3 arms | TODO | |
| E3b | Powered set | TODO | |
| E4 | Subscription value + dispatch simulation | TODO | |
| GATE1 | Gate 1 decision (deadline 2026-11-11) | TODO | |

## S4 Gateway (P1; PASS, or PROVISIONAL in shadow mode only)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S4-1 | Provider interface replaces Backend | TODO | |
| S4-2 | Anthropic Messages ingress | TODO | |
| S4-3 | Egress: Anthropic native, OpenAI-compatible, byte-stable pass-through | TODO | |
| S4-4 | Sticky session routing | TODO | |
| S4-5 | Failover + circuit breaker + typed errors | TODO | |
| S4-6 | Budgets, shadow mode, why, off/on, fail-open | TODO | |
| S4-7 | OTel GenAI attributes + route reason | TODO | |
| E5 | Gateway overhead | TODO | |
| E6 | Fault injection | TODO | |
| E7 | Cache preservation | TODO | |
| E8 | Conformance fixtures | TODO | |
| E9 | Security tests, govulncheck, SBOM | TODO | |

## S5 Delegate engine (P2; Gate 1 PASS only)

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S5-1 | SQLite step log, crash-resume test | TODO | |
| S5-2 | Bundles, verify allowlist, sandboxed verify | TODO | |
| S5-3 | Subscription-first dispatch | TODO | |
| GATE2 | Gate 2 decision | TODO | |

## S6 Education document

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| S6 | docs/EDUCATION.md | TODO | |

## Log

- 2026-10-17: S0 done. Baseline green.
