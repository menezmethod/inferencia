#!/usr/bin/env bash
# Run one matrix cell: one task, one model, solo, then grade it.
#
# Usage: run_cell.sh <workdir> <task-id> <model> <matrix.csv>
#   <workdir> is the build_suite.sh workdir (suite.tsv and the clones live there).
#   Run it from a copy of this directory, never from the repo, so an edit to the
#   repo cannot change a script while it runs.
#
# This is E3's run_arm.sh arm A with one change: the model is a parameter.
# Every model runs in the same harness (Claude Code, same prompt, tools, sandbox,
# turn limit and time limit), so the model is the only difference between cells.
#  - claude-* models use the user's Claude plan.
#  - Other models use the local LiteLLM gateway (Command Code) through
#    ANTHROPIC_BASE_URL. Its token is read from a file here, outside the sandbox.
#
# Grading is E3's, plus one extra signal recorded first: visible_pass, the result
# of go build and the task's packages with the files exactly as the agent left
# them. That is the failure signal a real router has (cascade-visible). Then the
# whole test tree is restored and the hidden grade runs (accepted).
set -euo pipefail

workdir=$1 id=$2 model=$3 results=$4
limit_s=${EVAL_TIME_LIMIT_S:-2700}
gateway=${EVAL_GATEWAY_URL:-http://127.0.0.1:4000}
gateway_key_file=${EVAL_GATEWAY_KEY_FILE:-$HOME/.config/commandcode/litellm-master}
here=$(cd "$(dirname "$0")" && pwd)
row=$(awk -F'\t' -v id="$id" '$1 == id' "$workdir/suite.tsv")
[[ -n "$row" ]] || { echo "no task $id" >&2; exit 2; }
IFS=$'\t' read -r _ repo commit parent pkgs verdict regress <<<"$row"
[[ "$verdict" == valid ]] || { echo "task $id is $verdict" >&2; exit 2; }

# Gateway model aliases (cc-delegate/litellm/config.yaml) and data rules.
case "$model" in
  claude-*) via=plan ;;
  deepseek-v4-1-flash) via=gateway ;;
  muse-spark-1.3-contributor)
    via=gateway
    # The provider may train on prompts: public code only.
    [[ "$(basename "$repo")" == inferencia ]] || { echo "skip $id $model: not a public repo" >&2; exit 4; } ;;
  *) echo "unknown model $model (add it here and to the catalog)" >&2; exit 2 ;;
esac
alias_of() { case "$1" in muse-spark-1.3-contributor) echo openai-muse-spark-1-3-contrib ;; *) echo "$1" ;; esac; }

cell="$id-$model"
run="$workdir/runs/$cell"
logs="$workdir/logs/$cell"
gocache="$workdir/gocache-$cell"
mkdir -p "$workdir/logs"
rm -rf "$run" "$gocache" "$logs".* && mkdir -p "$run" "$gocache"
git -C "$repo" archive "$parent" | tar -x -C "$run"

spec=$(git -C "$repo" diff --name-only --diff-filter=AM "$parent" "$commit" | grep -E '(_test\.go$|(^|/)testdata/)' || true)
[[ -n "$spec" ]] && git -C "$repo" archive "$commit" -- $spec | tar -x -C "$run"
git -C "$run" init -q && git -C "$run" add -A
git -C "$run" -c user.email=eval@local -c user.name=eval commit -qm "task start"

goroot=$(go env GOROOT)
modcache=$(go env GOMODCACHE)
tests=$(printf '%s\n' $spec | grep '_test\.go$' || true)
message=$(git -C "$repo" log -1 --format=%B "$commit")
# The prompt is E3's arm A prompt, word for word.
prompt="You are in a Go repository. Implement this change:

$message

These test files describe the required behavior. They must pass, and you must not edit them:
$(printf '  %s\n' $tests)
Work only in this directory. Keep existing behavior unless the change requires otherwise.
Before you finish, run go build ./... and go test on the packages above. The network is off;
every dependency you need is already in the module cache.
Do not ask questions; make reasonable choices. When the change is done, reply DONE."

settings=$(python3 -I - "$run" "$workdir" "$goroot" "$modcache" "$gocache" <<'PY'
import json, sys
run, workdir, goroot, modcache, gocache = sys.argv[1:]
print(json.dumps({"sandbox": {
    "enabled": True, "failIfUnavailable": True, "allowUnsandboxedCommands": False, "autoAllowBashIfSandboxed": True,
    "filesystem": {"denyRead": ["~/", workdir], "allowRead": [run, goroot, modcache, gocache], "allowWrite": [run, gocache]},
    "network": {"allowedDomains": [], "strictAllowlist": True}}}))
PY
)
tools=(Bash "Edit(./**)" Task)

profile="$logs.sb"
cat > "$profile" <<EOF
(version 1)
(allow default)
(deny file-read* file-write*
  (subpath "$HOME/.ssh") (subpath "$HOME/code") (subpath "$HOME/.claude/projects")
  (literal "$HOME/.claude.json") (subpath "$workdir"))
(allow file-read* file-write* (subpath "$run") (subpath "$gocache"))
EOF

envs=(GOCACHE="$gocache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local "PATH=$goroot/bin:$PATH")
if [[ $via == gateway ]]; then
  [[ -r "$gateway_key_file" ]] || { echo "no gateway key at $gateway_key_file" >&2; exit 2; }
  a=$(alias_of "$model")
  # Every model slot points at the same alias, so no call in this cell reaches another model.
  envs+=("ANTHROPIC_BASE_URL=$gateway" "ANTHROPIC_AUTH_TOKEN=$(cat "$gateway_key_file")"
    "ANTHROPIC_MODEL=$a" "ANTHROPIC_DEFAULT_OPUS_MODEL=$a" "ANTHROPIC_DEFAULT_SONNET_MODEL=$a"
    "ANTHROPIC_DEFAULT_HAIKU_MODEL=$a" "CLAUDE_CODE_SUBAGENT_MODEL=$a" CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1)
  model_arg=$a
else
  # Subagents and background calls stay on the cell's model too.
  envs+=("CLAUDE_CODE_SUBAGENT_MODEL=$model" CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1 "ANTHROPIC_DEFAULT_HAIKU_MODEL=$model")
  model_arg=$model
fi

start=$(date +%s)
set +e
(cd "$run" && env "${envs[@]}" "$here/group_run.sh" "$limit_s" \
  claude -p "$prompt" --model "$model_arg" --output-format stream-json --verbose \
  --max-turns 80 --permission-mode acceptEdits --allowedTools "${tools[@]}" \
  --no-session-persistence --setting-sources project --settings "$settings" \
  --mcp-config '{"mcpServers":{}}' --strict-mcp-config \
  > "$logs.stream.jsonl" 2> "$logs.stderr")
exit_code=$?
set -e
secs=$(( $(date +%s) - start ))
git -C "$run" add -A >/dev/null 2>&1 && git -C "$run" diff --cached --stat > "$logs.diffstat" 2>/dev/null || true

sandboxed_go() { # log, then the shell command
  local log=$1; shift
  (cd "$run" && sandbox-exec -f "$profile" env -i "PATH=$goroot/bin:/usr/bin:/bin" "HOME=$run" "GOCACHE=$gocache" \
    "GOMODCACHE=$modcache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local CGO_ENABLED=0 \
    sh -c "$*" > "$log" 2>&1)
}

# Visible signal: the files as the agent left them, task packages only.
visible_pass=0
sandboxed_go "$logs.visible.log" "go build ./... && go test -count=1 $pkgs" && visible_pass=1

# Hidden grade: reset the whole test suite to the commit, then build and test.
find "$run" -name '*_test.go' -not -path '*/.git/*' -delete
find "$run" -type d -name testdata -not -path '*/.git/*' -prune -exec rm -rf {} +
suite_files=$(git -C "$repo" ls-tree -r --name-only "$commit" | grep -E '(_test\.go$|(^|/)testdata/)' || true)
[[ -n "$suite_files" ]] && git -C "$repo" archive "$commit" -- $suite_files | tar -x -C "$run"
regress_pkgs=$(tr ',' ' ' <<<"$regress")
accepted=0
# Regression packages get up to three runs (a known flaky test in old commits, as in E3).
sandboxed_go "$logs.grade.log" "go build ./... && go test -count=1 $pkgs && { go test -count=1 $regress_pkgs || go test -count=1 $regress_pkgs || go test -count=1 $regress_pkgs; }" && accepted=1

python3 -I "$here/parse_cell.py" "$logs.stream.jsonl" "$id" "$model" "$accepted" "$visible_pass" "$secs" "$exit_code" "$run" >> "$results"
rm -rf "$gocache"
echo "$id $model accepted=$accepted visible=$visible_pass secs=$secs exit=$exit_code"
