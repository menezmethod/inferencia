# Gauntlet decisions

## D1. OpenCode Go bills flat (2026-10-17)

OpenCode Go is a flat monthly plan: Go $10/month, Go Plus $40/month. Each model has a monthly
allowance in dollars (for example Kimi K3: $15 on Go, $60 on Go Plus), with a 5-hour window of
20% and a weekly window of 50% of that allowance. The per-call `cost` OpenCode logs for
`opencode-go` is therefore notional: it draws down the allowance, it is not billed. With "Use
balance" on, usage past the allowance falls back to pay-as-you-go credit.
Source: https://opencode.ai/docs/go/ (read 2026-10-17).
Consequence: the ledger stores OpenCode Go dollars as `harness-computed` with billing `plan`,
and E4 can compute value per dollar paid as notional dollars / plan price.

## D2. No SQLite driver dependency for `measure` (2026-10-17)

`measure` computes its report from the harness logs on each run. It keeps no state, so it needs
no ledger database yet. To read OpenCode's `opencode.db` it runs the system `sqlite3` CLI in
read-only mode with JSON output. This adds no Go dependency. Ceiling: it needs `sqlite3` on the
PATH (present on macOS and most Linux). Upgrade path: when the gateway needs a persistent
ledger (S4), add a pure-Go driver (`modernc.org/sqlite`) and read OpenCode through it.

## D3. Golden-number checks are opt-in tests (2026-10-17)

The golden numbers read files in the user's home directory, which CI does not have. Unit tests
use synthetic fixtures and always run. The golden check runs only when
`INFERENCIA_GOLDEN=1` is set, and skips otherwise. This keeps `make test` hermetic.

## D4. Reading of two gauntlet lines (2026-10-17)

- "S4 GATEWAY (only after Gate 1 PASS)" is read together with the Gate 1 PROVISIONAL rule: S4
  runs on PASS, or on PROVISIONAL in shadow mode only.
- The second "FAIL:" line under Gate 1 is a duplicate of the first; both mean skip S4 and S5,
  then do S6.

## D5. Claude Code transcript retention loses ledger history (2026-10-17)

Finding F1 (evidence/S1.md): the Claude transcripts on disk cover only 23 days up to
2026-09-30, so `measure` finds 6,713 calls where the 2026-09-30 analysis found 12,247. Claude
Code deletes old transcripts. Two fixes:
1. The user raises Claude Code's transcript retention setting (`cleanupPeriodDays`). The
   gauntlet does not change user settings; this is a recommendation in the final report.
2. When the ledger becomes persistent (S4, with the SQLite driver from D2), `measure` stores
   normalized Usage records (tokens, model names, times; no text) so history outlives the logs.

## D6. E3 tests are the visible spec, restored before grading (2026-10-17)

The smoke run showed that commit messages leave API shapes unstated: T01's hidden test expects a
field `Think *bool`, the message only says "support Ollama think control", and both arms built a
reasonable `json.RawMessage` instead. Grading unstated API shapes measures guessing. So each task
shows its test files as the spec ("must pass, do not edit"), and grading restores the original test
files before running them, so editing a test cannot make a task pass. All arms see the same
information, so the paired comparison stays fair. This departs from the gauntlet's "hidden tests";
the tests are still the grader.

## D7. Claude Code bills to a plan, and its stream reports quota (2026-10-17)

`claude -p --output-format stream-json` emits `rate_limit_event` records with the plan's 5-hour and
7-day utilization (0.21 and 0.10 during the smoke run). That settles finding "Claude billing
unknown": Claude Code runs on a Claude plan. It also gives subscription-first dispatch a quota
signal from the official client itself. The E3 pilot driver uses it: it sleeps when the 5-hour
window passes 85% and stops when the 7-day window passes 60%, to leave the user plan capacity.
