# E4 Subscription value

**Hypothesis.** Flat $10-$20 plans deliver many times their price in per-token value, and a
dispatcher that fills plans before paying per token would have avoided real per-token spend.

**Method.** All from local logs; no model was called.
1. Value per dollar paid: `inferencia subs --until 2026-10-17` (catalog list prices), and for
   OpenCode Go also OpenCode's own computed dollars, which is what drains its allowance.
2. Plan fit: OpenCode Go usage per model per month against its published monthly limits
   (opencode.ai/docs/go, read 2026-10-17); Codex user turns per rolling 5-hour window against the
   ChatGPT Plus ranges (learn.chatgpt.com/docs/pricing, read 2026-10-17).
3. Avoidable per-token spend: pay-per-token calls whose model was included in a plan the user held,
   in a month where that plan's allowance for the model was unused.

Commands: `sqlite3 -readonly` aggregates over `opencode.db` (message table only), a Python pass over
Codex `task_started` events in main (non-subagent) sessions, and `inferencia measure`/`subs`. Numbers
are in [data.csv](data.csv).

## Result

### Value per dollar paid (list-price equivalent, from S2)

| Plan | Best month | Value | vs price |
| --- | --- | --- | --- |
| ChatGPT (Codex) | 2026-10 (partial) | $602.71 | Plus $20: 30.1x; Pro $100: 6.0x |
| Claude (if on a plan) | 2026-10 (partial) | $628.00 | Pro $20: 31.4x; Max $100: 6.3x |
| OpenCode Go | 2026-08 | $33.04 list / $13.73 at OpenCode's prices | Go $10: 3.3x list / 1.4x own |

### Which tier you hold

- **OpenCode Go: almost certainly Go ($10).** In July, Kimi K3 used $15.05 of allowance at OpenCode's
  prices and stopped. Kimi K3's monthly limit is $15 on Go and $60 on Go Plus.
- **ChatGPT: not decidable from the logs.** Peak use was 14 gpt-6-astra turns in a 5-hour window.
  Plus allows 5-45 astra messages per 5 hours, depending on message size; these turns carry very
  large contexts, which points to the low end, so Plus would likely throttle at peak. Pro has no
  5-hour limit.

### Avoidable per-token spend (subscription-first, from the logs)

| Month | Paid per token | $ | Plan that already covered it |
| --- | --- | --- | --- |
| 2026-07 | OpenRouter z-ai/glm-5.2, 333 calls | 7.41 | OpenCode Go: GLM-5.2 $60/month allowance, unused in July |
| 2026-10 | Command Code muse-spark-1.3-contributor + deepseek-v4.1-flash, 504 calls | 3.32 | OpenCode Go: both included ($60, $80), no Go usage in October |
| **Total** | | **10.73** | |

The July case is the pattern subscription-first dispatch exists for: the Kimi K3 allowance ran out,
work moved to a paid route, and an included model with an unused $60 allowance sat idle.

## What it shows

1. Flat plans are the cheapest tokens you buy, by a wide margin, when you use them: 25-31x list
   value on $20 plans at your volume.
2. The avoidable spend found is small in dollars ($10.73 over four months) because most of your
   paid-per-token use is already inside plans. The mechanism is real; the dollars at your volume
   are not large.
3. The large number is the other direction: work that runs on plans now is worth $600+ a month at
   list prices. The risk to manage is plan limits, not per-token spend.

## Threats to validity

- List prices and harness price tables, not bills (E1 skipped).
- Plan limits are published ranges and estimates; real throttling depends on message size and the
  provider's current rules.
- Claude Code billing (plan or API key) is not in the logs.
- Claude Code deleted transcripts before July 2026 (F1).
- "Avoidable" assumes the included model would have done the work equally well.
