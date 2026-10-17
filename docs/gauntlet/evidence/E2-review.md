# E2 adversarial review (commit 08998f1) and fixes

Reviewer: fresh `ecc:go-reviewer` subagent. No code BLOCKER; 5 MAJOR, 7 MINOR.

| Finding | Fix | Proof |
| --- | --- | --- |
| M1 interleaved subagents in one session merged into one conversation | `ledger.Usage.Agent` (the subagent file name) is part of the conversation key | `TestInterleavedSubagentsAreSeparateConversations` |
| M2 subagent test was vacuous | rewritten so a merged key would count 2 switches | mutation check: removing the key split fails both subagent tests |
| M3 "Mode B ceiling 10%" overclaimed | README: point estimate for the targets tried (0.89x-0.90x; opus 1.08x) | sensitivity runs in output.txt |
| M4 savings read as real savings | README "What the dollars mean": list-price value, flat-plan calls cost $0 at the margin | - |
| M5 "trap expensive only for expensive targets" untested | sensitivity run with claude-opus-5-5 as target: $233 loss, 1.16x | data.csv |
| m1 1-hour writes re-written at the 5-minute price | re-written tokens go to the 1-hour cache when the call used it | `TestOneHourCacheIsRewrittenAtTheOneHourPrice` |
| m1 aliases counted as switches | compare catalog ids | `TestAliasIsNotASwitch` |
| m1 switch-back and no-write-price target untested | tests added | `TestSwitchBackIsChargedEachTime` |
| m2 typo in --strong/--fast printed a $0 row | unknown target exits 1 | `TestRunReplayRejectsUnknownTarget` |
| m3 multi-call (Codex) records counted N switches | only single-call records can switch; 1 per switch | `TestMultiCallRecordsNeverSwitch` |
| m5 "cheapest policy" for subscription-first | README: lower bound only, not a real policy | - |
| m6 unexplained sensitivity row | output.txt and data.csv label the sensitivity runs | - |
| m7 naive loss only checked > 0 | exact value asserted | `TestNaivePerCallSwitchesAtThreshold` |

Effect on numbers: the naive router's switches fell from 547 to 308 (no more fake switches between
interleaved subagents or inside Codex records); all other default rows are unchanged.
