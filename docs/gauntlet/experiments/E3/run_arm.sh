#!/usr/bin/env bash
# Run one E3 task in one arm, then grade it with the hidden tests.
#
# Usage: run_arm.sh <workdir> <task-id> <arm A|B|C> <results.csv>
#   <workdir> is the build_suite.sh workdir (it holds suite.tsv and the clones).
#
# The task repo is the parent commit's files in a fresh git repo with no
# history, so the answer commit cannot be read from git. The arm runs through
# the official client (`claude -p`) under the user's plan: no inferencia in
# the request path (hard rule 2). Grading copies the commit's test files in
# and runs them in the build_suite.sh sandbox.
#
# Arms:
#   A  solo: claude-opus-5-5, default subagent and background models
#   B  routed by harness config: same, but subagents forced to claude-sonnet-5-5
#      and background work on claude-haiku-5-5 (CLAUDE_CODE_SUBAGENT_MODEL[_FORCE],
#      ANTHROPIC_DEFAULT_HAIKU_MODEL)
#   C  delegated: claude-opus-5-5 plans bundles and sends them to cc-delegate
#      workers (deepseek-v4-1-flash through its local gateway); it runs the
#      verify commands itself
set -euo pipefail

workdir=$1 id=$2 arm=$3 results=$4
limit_s=${E3_TIME_LIMIT_S:-2700}
row=$(awk -F'\t' -v id="$id" '$1 == id' "$workdir/suite.tsv")
[[ -n "$row" ]] || { echo "no task $id" >&2; exit 2; }
IFS=$'\t' read -r _ repo commit parent pkgs verdict <<<"$row"
[[ "$verdict" == valid ]] || { echo "task $id is $verdict" >&2; exit 2; }

run="$workdir/runs/$id-$arm"
rm -rf "$run" && mkdir -p "$run"
git -C "$repo" archive "$parent" | tar -x -C "$run"
git -C "$run" init -q && git -C "$run" add -A
git -C "$run" -c user.email=e3@local -c user.name=e3 commit -qm "task start"

spec=$(git -C "$repo" log -1 --format=%B "$commit")
prompt="You are in a Go repository. Implement this change:

$spec

Work only in this directory. Keep existing behavior unless the change requires otherwise.
Add or update tests where useful. Before you finish, run go build ./... and go test ./... .
Do not ask questions; make reasonable choices. When the change is done, reply DONE."

tools=(Read Edit Write Glob Grep Task "Bash(go:*)" "Bash(gofmt:*)" "Bash(ls:*)" "Bash(git diff:*)" "Bash(git status:*)")
envs=()
extra=()
case "$arm" in
  A) ;;
  B) envs=(CLAUDE_CODE_SUBAGENT_MODEL=claude-sonnet-5-5 CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1 ANTHROPIC_DEFAULT_HAIKU_MODEL=claude-haiku-5-5) ;;
  C)
    printf '{"mcpServers":{"delegate-local":{"command":"%s"}}}\n' "$HOME/AI/cc-delegate/bin/run-mcp.sh" > "$run.mcp.json"
    extra=(--mcp-config "$run.mcp.json" --strict-mcp-config)
    tools+=("mcp__delegate-local__delegate_batch" "mcp__delegate-local__delegate_to_local_agent")
    prompt="$prompt

How to work: plan once, then split the change into 1-6 bundles. Send the bundles to workers with
mcp__delegate-local__delegate_batch (model deepseek-v4-1-flash, workdir $run). Give each bundle its goal,
the files in scope, constraints, the verify command, and what done means. Workers never certify their own
work: after they finish, run the verify commands yourself and review the diff. Write code yourself only to
fix small problems the workers leave."
    ;;
  *) echo "unknown arm $arm" >&2; exit 2 ;;
esac

start=$(date +%s)
set +e
# ${a[@]+"${a[@]}"}: an empty array is "unbound" under set -u in macOS bash 3.2.
(cd "$run" && env ${envs[@]+"${envs[@]}"} perl -e 'alarm shift; exec @ARGV' "$limit_s" \
  claude -p "$prompt" --model claude-opus-5-5 --output-format stream-json --verbose \
  --max-turns 80 --permission-mode acceptEdits --allowedTools "${tools[@]}" \
  --no-session-persistence --setting-sources project ${extra[@]+"${extra[@]}"} \
  > "$run.stream.jsonl" 2> "$run.stderr")
exit_code=$?
set -e
secs=$(( $(date +%s) - start ))

# Grade: put the hidden tests in place and run them in the sandbox.
for f in $(git -C "$repo" diff --name-only --diff-filter=AM "$parent" "$commit" -- '*_test.go'); do
  mkdir -p "$run/$(dirname "$f")"
  git -C "$repo" show "$commit:$f" > "$run/$f"
done
gobin="$(go env GOROOT)/bin/go"
accepted=0
(cd "$run" && env -i "PATH=$PATH" "HOME=$workdir/home" "GOCACHE=$workdir/gocache" \
  "GOMODCACHE=$(go env GOMODCACHE)" GOFLAGS=-mod=mod GOPROXY=https://proxy.golang.org \
  GOTOOLCHAIN=local CGO_ENABLED=0 "$gobin" test -count=1 $pkgs > "$run.grade.log" 2>&1) && accepted=1

# Usage from the stream: the final result event, plus worker usage from cc-delegate tool results.
python3 -I - "$run.stream.jsonl" "$id" "$arm" "$accepted" "$secs" "$exit_code" >> "$results" <<'PY'
import json, sys
path, tid, arm, accepted, secs, code = sys.argv[1:]
res, worker = {}, {"input": 0, "cache_read": 0, "output": 0}
def walk(o):
    if isinstance(o, dict):
        u = o.get("usage")
        if isinstance(u, dict) and "prompt_tokens" in u:  # OpenAI-shape usage from cc-delegate workers
            worker["input"] += u.get("prompt_tokens", 0)
            worker["output"] += u.get("completion_tokens", 0)
            worker["cache_read"] += (u.get("prompt_tokens_details") or {}).get("cached_tokens", 0)
        for v in o.values(): walk(v)
    elif isinstance(o, list):
        for v in o: walk(v)
for line in open(path, errors="replace"):
    try: ev = json.loads(line)
    except ValueError: continue
    if ev.get("type") == "result": res = ev
    if ev.get("type") == "user":
        for block in (ev.get("message") or {}).get("content") or []:
            if isinstance(block, dict) and block.get("type") == "tool_result":
                for part in block.get("content") or []:
                    text = part.get("text") if isinstance(part, dict) else None
                    if text:
                        try: walk(json.loads(text))
                        except ValueError: pass
u = res.get("usage") or {}
print(",".join(str(x) for x in [tid, arm, accepted, secs, code, res.get("num_turns", ""), res.get("total_cost_usd", ""),
    u.get("input_tokens", ""), u.get("cache_read_input_tokens", ""), u.get("cache_creation_input_tokens", ""), u.get("output_tokens", ""),
    worker["input"], worker["cache_read"], worker["output"], res.get("subtype", "none")]))
PY
echo "$id $arm accepted=$accepted secs=$secs exit=$exit_code"
