# E2 Offline replay

**Hypothesis.** A router that changes which model runs some calls would have cut the list-price
cost of the recorded sessions, after paying for the cache it breaks.

**Method.** `inferencia replay --until 2026-10-17T00:00:00Z` re-prices every recorded call under
routing policies at catalog list prices (as of 2026-10-17). No model is called. Each call keeps its
token split. All policies run on the same 23,732 calls; 12 calls that cannot be priced as run are
excluded from all of them. A cache loss is charged when a policy switches model inside a
conversation where the recorded run did not: the tokens the old model read from cache are
written again on the new model (at the 1-hour price when the recorded call used the 1-hour cache).
A conversation is harness + session + subagent, so Claude Code subagents never merge with their
parent or with each other. Models compare by catalog id, so an alias is not a switch. Codex
records are whole sessions and never switch.

Reproduce: `go run ./cmd/inferencia replay --until 2026-10-17T00:00:00Z`, plus `--strong
claude-opus-5-5` and `--strong deepseek-v4.1-flash` for the sensitivity runs (full output in
[output.txt](output.txt), numbers in [data.csv](data.csv)).

**What the dollars mean.** "As-run" is the list-price value of all usage, not what you paid. Much of
it ran under flat plans (Codex, OpenCode Go) whose extra cost per call was $0, and Claude Code's
billing is unknown. So every "vs as-run" below compares list-price value with list-price value. It
says which routing uses fewer per-token dollars; it is not your bill going down by that share.

## Result (estimates at list prices)

| Policy | Est. $ | vs as-run | Switches | Cache loss $ |
| --- | --- | --- | --- | --- |
| as-run | 1,849.05 | 1.00x | 0 | 0.00 |
| delegated work -> deepseek-v4-pro | 1,655.52 | 0.90x | 0 | 0.00 |
| main loop off frontier -> deepseek-v4-pro | 750.11 | 0.41x | 0 | 0.00 |
| main loop off frontier -> claude-opus-5-5 (same tier) | 1,414.51 | 0.76x | 0 | 0.00 |
| naive per call: <128k -> muse-spark, else deepseek-v4-pro | 136.94 | 0.07x | 308 | 7.07 |
| naive per call: <128k -> muse-spark, else claude-opus-5-5 | 2,153.60 | 1.16x | 308 | 232.93 |
| subscription-first: plan fees (lowest to highest tier) + per-token rest | 132.90 - 1,968.90 | 0.07x - 1.06x | 0 | 0.00 |

## What it shows

1. **Delegation alone moves about 10% for the targets tried.** Moving delegated work to
   deepseek-v4-pro (0.90x) or deepseek-v4.1-flash (0.89x) saves about 10% of list-price value;
   moving it to claude-opus-5-5 costs 8% more. This is a point estimate for those targets, not a
   ceiling: it cannot see delegation shrinking the main loop's context, which E3 can.
2. **The main-loop model is the lever.** Moving solo frontier calls to a strong-tier model gives
   0.39x-0.41x. Even staying in the frontier tier and moving to its cheapest member
   (claude-opus-5-5, whose cache reads cost $0.20/MTok against gpt-6-astra's $1.00) gives 0.76x,
   with no drop in tier.
3. **The cache trap depends on the target's price.** The same naive router switches 308 times in
   both runs. With a cheap large-call target it loses $7 to re-written caches; with
   claude-opus-5-5 as the target it loses $233 and ends at 1.16x, more than doing nothing. A router
   that switches models mid-conversation can cost more than no router at all.
4. **Subscription-first spans 15x and its low end is not a real policy.** The low end assumes every
   plan-covered call fits in the cheapest tier of each plan, which this volume does not (E4
   estimates it from quotas). It is a lower bound only.

## Threats to validity

- List prices, not bills (E1 was skipped: no billed source), and flat-plan calls cost $0 at the
  margin, as said above.
- Same token counts on a different model: tokenizers differ (Anthropic states about 30% more
  tokens for its newer tokenizer), and a cheaper model may need more turns or fail.
- Quality is not modeled. A policy is only worth it if E3 shows its accept rate holds.
- Re-writing the whole cache on every switch is conservative when a switch-back lands inside the
  old model's cache lifetime.
- Codex records are whole sessions: the naive router sees one average prompt per Codex session
  and cannot switch inside one.
- Default targets were picked as the cheapest per tier on this same usage: best cases.
- Claude Code transcripts before July 2026 were deleted by Claude Code (finding F1).
