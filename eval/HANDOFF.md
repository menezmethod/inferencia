# Handoff: routing evaluation (2026-10-22)

**Goal.** Inferencia is a lightweight inference router. For each request, it selects the
lowest-cost provider that can do the job well, and it needs almost no configuration.

**Where things are.**

- Branch `eval/routing-matrix`, PR #72 (open, CI green, not merged). `main` is not changed.
- The spec is [README.md](README.md). Read it first. It is frozen: do not change a gate, a
  decider rule, a Laya question or the 0.5 threshold. If a change is necessary, record it in
  "Changes after freeze" with the reason.
- Old plans are in `docs/archive/` (delete the folder when you want; `git rm` was blocked in
  the cloud session).

**What is done.**

- Matrix harness, cost parser, analysis and deciders, with 32 unit tests (`make eval-test`).
- An independent review; its fixes are in commit 869574e.
- Laya 0.2.0 ran on the 16 public prompts: 3.6-9.1 s per decision on a cloud CPU; `fast_ok`
  0.63-0.80 on 15 of 16 tasks. Recorded in the spec.

**What is next (on the Mac).** Follow [matrix/RUN.md](matrix/RUN.md) in order:

1. Start the gateway: `~/AI/cc-delegate/bin/start-gateway.sh`.
2. Smoke test: 3 cells (Haiku, DeepSeek, Muse on T01). Check every item in RUN.md step 2
   before you continue. Stop if a gateway row names a Claude model.
3. Full matrix: 109 new cells. It stops at 60% of the 7-day Claude window or $10 of Command
   Code spend. The Command Code month resets on 2026-10-30 (66% was used on 2026-10-22).
4. Deciders on the Mac (this gives the real G3 latency for Laya).
5. Ledger check (G4) against the Command Code dashboard.
6. Analysis: `report.md` in `eval/results/`.
7. Only then write `docs/PLAN_V5.md`, with only the layers that the report supports.

**Rules.**

- Never commit `features.jsonl` or stream logs: they contain private-repo commit messages.
  Commit only `eval/results/matrix.csv`, `decisions-*.csv` and `report.md`.
- Muse runs only on public inferencia tasks (the provider may train on prompts).
- Do not push to `main`: a merge to `main` deploys through Coolify. Push to the branch.
- An invalid run (API or gateway error) is not a model failure. The runner removes it and
  stops. Fix the cause, then rerun the same command.

**Open items.**

- Jev is not tested: there is no TypeSafe API key.
- No held-out repo yet. Add a third repo only if a decider passes G1.
