# E3 Live paired A/B/C pilot

**Hypothesis.** Routing (arm B) or delegating to cheap workers (arm C) lowers cost per accepted task
against a solo frontier model (arm A) without lowering the accept rate.

**Method.**
- Suite: 31 feature commits from the user's own repos (inferencia and one private repo), validated by
  [build_suite.sh](build_suite.sh): the commit's tests fail at the parent and pass at the commit 3 runs
  in a row ([suite.tsv](suite.tsv)).
- Each task starts as the parent's files in a fresh repo with no history; the commit's test files and
  testdata are the visible spec (D6).
- Arms run through the official `claude -p` under the user's Claude plan, in a random order, one at a
  time ([run_arm.sh](run_arm.sh), [run_pilot.sh](run_pilot.sh)).
- Isolation: Claude Code's OS sandbox for shell commands, `sandbox-exec` for cc-delegate, no network, a
  leak check; verified by a live probe ([review](../../evidence/E3-harness-review.md)).
- Grading: the commit's whole test tree restored, then `go build ./...`, the task's packages and the
  regression set.
- Cost: the client's API-equivalent `total_cost_usd`, plus worker tokens from cc-delegate's dispatch
  log at catalog prices ([parse_run.py](parse_run.py), [analyze.py](analyze.py)).
- Quota guard: stop at 60% of the plan's 7-day window. The run ended at 21%.

Reproduce: `build_suite.sh <dir> candidates.tsv`, then `run_pilot.sh <dir> <dir>/pilot.csv`, then
`reparse.sh` and `analyze.py`. Results: [results/pilot.csv](results/pilot.csv).

**Result.** See [GATE1.md](../../GATE1.md). C matched A on quality (90% vs 87%, non-inferior at 10
points) and on cost ($0.86 vs $0.86 per accepted task; ratio 1.01, 95% CI 0.84-1.17). B equaled A,
because no B run used a subagent. Delegation cost 1.24x on the smallest third of tasks and 0.99x on
the largest third.

**Threats to validity.** Small tasks; two repos; visible tests; two unsolvable tasks; API-equivalent
cost, not a bill; one orchestrator and one worker model. A first pilot run was lost to an app restart
after 11 tasks; its interim numbers pointed the same way.
