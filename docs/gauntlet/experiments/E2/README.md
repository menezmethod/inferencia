# E2 Offline replay

**Hypothesis.** A router that changes which model runs some calls would have cut the cost of the
recorded sessions, after paying for the cache it breaks.

**Method.** `inferencia replay --until 2026-10-17T00:00:00Z` re-prices every recorded call under
five policies at catalog list prices (as of 2026-10-17). No model is called. Each call keeps its
token split. All policies run on the same 23,732 calls; 12 calls that cannot be priced as run are
excluded from all of them. A cache loss is charged when a policy switches model inside a session
where the recorded run did not: the tokens the old model read from cache are written again on the
new model. Claude Code subagent calls count as their own session.

Reproduce: `go run ./cmd/inferencia replay --until 2026-10-17T00:00:00Z` (full output in
[output.txt](output.txt), numbers in [data.csv](data.csv)).

## Result (estimates)

| Policy | Est. $ | vs as-run | Mid-session switches | Cache loss $ |
| --- | --- | --- | --- | --- |
| as-run | 1,849.05 | 1.00x | 0 | 0.00 |
| delegated work -> deepseek-v4-pro | 1,655.52 | 0.90x | 0 | 0.00 |
| main loop off frontier -> deepseek-v4-pro | 750.11 | 0.41x | 0 | 0.00 |
| naive per call (<128k prompt -> muse-spark-1.3-contributor, else deepseek-v4-pro) | 143.07 | 0.08x | 547 | 13.19 |
| subscription-first (plan fees at lowest to highest tier, + per-token rest) | 132.90 - 1,968.90 | 0.07x - 1.06x | 0 | 0.00 |

## What it shows

1. **Moving only delegated work saves 10%.** Subagents carry a small share of spend, so the
   plan's Mode B ceiling on these logs is about 10% unless delegation also shrinks the main loop's
   context (which this replay cannot show; E3 can).
2. **The main-loop model is the lever.** Moving solo frontier calls to a strong-tier model would
   cut cost by 59%, as the plan predicted from cache-read share.
3. **The cache trap is real but cheap at these prices.** The naive router switches models 547
   times mid-session, yet the switches cost only $13, because the cheap targets charge little to
   re-write a cache. The trap is expensive only when the target model is expensive. The larger
   risk of a naive router is quality and continuity (thinking and tool state), which a price
   replay cannot see.
4. **Subscription-first spans 15x.** At the lowest plan tiers it is the cheapest policy ($133);
   at the highest it is the most expensive ($1,969). Which is real depends on the tiers needed to
   carry this volume, which E4 estimates from quotas.

## Threats to validity

- List prices, not bills (E1 was skipped: no billed source).
- Same token counts on a different model: tokenizers differ (Anthropic states about 30% more
  tokens for its newer tokenizer), and a cheaper model may need more turns or fail.
- Quality is not modeled at all. A policy is only worth it if E3 shows its accept rate holds.
- Codex records are whole sessions, so the naive router sees one average prompt per Codex session.
- The default targets were chosen as the cheapest per tier on this same usage: best cases.
- Claude Code transcripts before July 2026 were deleted by Claude Code (finding F1).
