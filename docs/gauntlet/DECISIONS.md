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
