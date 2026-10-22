# Run the routing matrix (Mac)

The harness needs macOS (`sandbox-exec` and the Claude Code sandbox), the repos, and your
Claude plan. Run every step in Terminal on the Mac.

## 1. Before you start

- Go and the `claude` CLI are installed and signed in to your Claude plan.
- The E3 workdir from the pilot exists (it has `suite.tsv` and the clones). If not, build it:
  `REPOS_ROOT=~/code docs/gauntlet/experiments/E3/build_suite.sh ~/eval-work docs/gauntlet/experiments/E3/candidates.tsv`.
  For `private-repo-b`, symlink that name under `REPOS_ROOT` to the real private repo.
- The Command Code gateway runs (keep this terminal open):

  ```bash
  ~/AI/cc-delegate/bin/start-gateway.sh
  ```

- Check the gateway with one call:

  ```bash
  curl -s http://127.0.0.1:4000/v1/messages -H "x-api-key: $(cat ~/.config/commandcode/litellm-master)" \
    -H "anthropic-version: 2023-06-01" -H "content-type: application/json" \
    -d '{"model":"deepseek-v4-1-flash","max_tokens":16,"messages":[{"role":"user","content":"Say OK"}]}'
  ```

## 2. Smoke test: one cell per gateway model

Run this first. It costs a few cents. Read both rows before you continue.

```bash
cd ~/code/inferencia            # your clone, on branch eval/routing-matrix
rm -rf /tmp/evalrun && cp -R eval/matrix /tmp/evalrun
W=~/eval-work                    # the E3 workdir
/tmp/evalrun/run_cell.sh $W T01 deepseek-v4-1-flash $W/smoke.csv
/tmp/evalrun/run_cell.sh $W T01 muse-spark-1.3-contributor $W/smoke.csv
cat $W/smoke.csv
```

Check for each row: `cost_source` is `model_usage` or `messages` (not `unpriced`), `models`
names only that cell's model, and `outside_paths` is 0. If `models` names a Claude model, the
gateway variables did not apply: stop and report it.

## 3. The full matrix

```bash
mkdir -p ~/code/inferencia/eval/results
EVAL_PILOT_CSV=~/code/inferencia/docs/gauntlet/experiments/E3/results/pilot.csv \
  EVAL_COPY_TO=~/code/inferencia/eval/results/matrix.csv /tmp/evalrun/run_matrix.sh $W $W/matrix.csv
```

- It imports the Opus column from the E3 pilot, then runs the missing cells in a random model
  order per task. Rerun the same command after any stop; finished cells are skipped.
- It stops at 60% of the 7-day Claude window, or at $10 of Command Code spend.
- Expected: 4 models x 31 tasks (Muse on public tasks only), about 105 cells.

## 4. Deciders

```bash
cd ~/code/inferencia/eval/deciders
python3 -I features.py $W > $W/features.jsonl
python3 -I rules.py < $W/features.jsonl > $W/decisions-rules.csv
(cd laya && npm ci && node decide.mjs < $W/features.jsonl > $W/decisions-laya.csv)
```

## 5. Ledger truth (G4)

```bash
cd ~/code/inferencia && go run ./cmd/inferencia measure --since 2026-09-30 | tee $W/measure.txt
```

Compare the `command-code` rows with the Command Code dashboard for the same billing month
(4,923 model requests and 295.7M tokens on 2026-10-22). The ledger reads only
`~/.commandcode/projects`. Calls through the LiteLLM gateway (cc-delegate workers and this
matrix) are on the bill but not in those logs, so name that gap with its size.

## 6. Analysis

```bash
cd ~/code/inferencia/eval/results
cp $W/matrix.csv $W/decisions-*.csv $W/features.jsonl .
python3 -I ../analysis/matrix.py matrix.csv ../../docs/gauntlet/experiments/E3/suite.tsv decisions-*.csv > report.md
```

Commit `eval/results/` (aggregates only; the stream logs stay in `$W/logs`). Then the
architecture plan is written from `report.md`.
