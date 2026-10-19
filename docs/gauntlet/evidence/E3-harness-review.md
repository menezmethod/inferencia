# E3 harness: adversarial review and fixes

Reviewer: fresh general-purpose subagent, files only (it ran no arms). 3 BLOCKER, 8 MAJOR, 5 MINOR.
All accepted. The smoke runs before these fixes (T01, T31) are discarded.

| # | Finding | Fix |
| --- | --- | --- |
| B1 | Answers reachable: clones checked out at HEAD, source repos, sibling runs, old streams; `git diff --no-index`; Read/Grep/Glob with absolute paths; unconfined worker shell in arm C | Clones made with `--no-checkout`; Claude Code Bash sandbox denies reads under `~/` and the workdir, re-opening only the run, Go toolchain and module cache; file tools have no blanket allow (reads outside the run are denied in -p mode); arms of a task run one at a time; cc-delegate runs under a `sandbox-exec` profile denying `~/code`, `~/.ssh`, `~/.claude/projects`, `~/.claude.json` and the workdir; `parse_run.py` flags any absolute path outside the run (`outside_paths`), and such tasks are excluded pending review |
| B2 | Agent-written code ran with the real HOME, network and `go env -w` | Bash sandbox: writes only to the run and its Go cache, no network (`strictAllowlist`, empty allowlist), no unsandboxed fallback, `failIfUnavailable`; grading runs under `sandbox-exec` with `env -i` and HOME set to the run |
| B3 | Timeouts and crashes cost $0 | No `result` event: cost summed from the assistant messages' usage at catalog prices, `cost_partial=1`; an empty cost is excluded and listed, never 0 |
| M4 | Testdata never copied or restored | Spec and grading include every `testdata/` file of the commit |
| M5 | A wrong solution could pass (edit other tests, add a `TestMain` that exits 0) | Grading deletes every `*_test.go` and `testdata/` in the run and restores the commit's whole test tree |
| M6 | No regression check | Grading requires `go build ./...`, the task's packages, and every package that passed at the commit (regression set recorded per task) |
| M7 | Grading used `-mod=mod`, validation `-mod=readonly` | `-mod=readonly` everywhere; arms and grading run offline (`GOPROXY=off`) on the module cache warmed by validation |
| M8 | Worker cost undercounted (errors, truncated results, failover priced as flash) | Worker usage read from cc-delegate's own dispatch log (`DELEGATE_LOG_PATH`), with `model_used`; models outside the price table are flagged |
| M9 | Orphans kept editing the run after a timeout | `group_run.sh`: own process group, whole group killed on timeout and on exit, before grading |
| M10 | A plan-limit stop counted as a quality failure | `quota_hit` column; the driver removes the task's rows and reruns it after a backoff |
| M11 | Quota guard failed open | Maximum across all streams of the task; no signal stops the pilot; null-safe; `resetsAt` seconds or milliseconds |
| M12 | Arms A/B could load user MCP servers | Every arm gets `--strict-mcp-config` (A/B with an empty config); the init event shows only built-in plugins and agents with `--setting-sources project` |
| M13 | No manipulation check for arm B | `models` column from `result.modelUsage`; analysis reports how many B runs used the configured subagent model |
| m14 | Alpha mismatch | Non-inferiority verdict uses the one-sided 5th percentile; sample size uses (delta + d)^2 |
| m15 | Bootstrap edge cases | Undefined ratios counted and dropped, zero-accept arms give infinity, n = 0 guarded, duplicate rows reported |
| m16 | Resume typo did nothing; no cleanup on stop | Unknown task id exits 2; `trap` kills arm jobs |
| m17 | Fixed arm order shares cache warmth | Random arm order per task, arms sequential |
| m18 | Flaky tests; committed suite unusable directly | Validation runs the commit's tests 3 times; the runner reads the workdir suite with full clone paths |

Isolation check from a real run: the init event lists `mcp_servers` = delegate-local only (arm C),
`plugins` = built-ins only, `agents` = built-ins only.

## Isolation probe (live, 2026-10-17)

One `claude -p` run with the exact arm settings was asked to try six things:

| Attempt | Result |
| --- | --- |
| Bash: read the source repo's go.mod under ~/code | denied: Operation not permitted |
| Bash: ls ~/.ssh | denied: Operation not permitted |
| Read tool on the source repo's go.mod | denied: permission not granted |
| Bash: ls the clones in the workdir | denied: Operation not permitted |
| Bash: curl https://example.com | denied: host is not on the allow list |
| Bash: write and read a file in the run directory | allowed |

Hardened smoke run: T01 arm A accepted in 40 s; the task's tests and the 9-package regression set
passed; no leak flags. Suite rebuilt: 31 valid tasks of 34 (3 pass at the parent).
