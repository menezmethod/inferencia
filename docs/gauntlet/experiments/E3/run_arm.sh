#!/usr/bin/env bash
# Run one E3 task in one arm, then grade it.
#
# Usage: run_arm.sh <workdir> <task-id> <arm A|B|C> <results.csv>
#   <workdir> is the build_suite.sh workdir (suite.tsv and the clones live there).
#   Run it from a copy of this directory, never from the repo, so an edit to the
#   repo cannot change a script while it runs.
#
# Isolation (hard rule 10, review blockers 1-2):
#  - The task repo is the parent commit's files in a fresh git repo with no history.
#  - Claude Code's Bash sandbox (OS-enforced) denies reads under ~/ and <workdir>,
#    re-opens only this run, the Go toolchain and the module cache, allows writes
#    only to this run and its Go cache, and allows no network.
#  - The file tools get no blanket allow, so in -p mode reads outside the run
#    directory are denied.
#  - Arm C's cc-delegate server, whose workers run shell commands, runs under a
#    sandbox-exec profile with the same denials.
#  - Grading runs under the same sandbox-exec profile with a clean environment.
#  - parse_run.py flags any absolute path outside the run (leak check).
#
# Spec and grading (D6): the commit's test files and testdata are the visible spec.
# Grading resets every test file and testdata directory to the commit, deletes
# test files the agent added, and requires: go build ./..., the task's packages,
# and every package that passed at the commit (the regression set).
#
# Arms: A solo claude-opus-5-5 | B same, subagents on claude-sonnet-5-5 and
# background on claude-haiku-5-5 | C opus plans, cc-delegate flash-builder
# workers (deepseek-v4-1-flash) build, opus verifies.
set -euo pipefail

workdir=$1 id=$2 arm=$3 results=$4
limit_s=${E3_TIME_LIMIT_S:-2700}
here=$(cd "$(dirname "$0")" && pwd)
row=$(awk -F'\t' -v id="$id" '$1 == id' "$workdir/suite.tsv")
[[ -n "$row" ]] || { echo "no task $id" >&2; exit 2; }
IFS=$'\t' read -r _ repo commit parent pkgs verdict regress <<<"$row"
[[ "$verdict" == valid ]] || { echo "task $id is $verdict" >&2; exit 2; }

run="$workdir/runs/$id-$arm"
logs="$workdir/logs/$id-$arm"
gocache="$workdir/gocache-$id-$arm"
mkdir -p "$workdir/logs"
rm -rf "$run" "$gocache" "$logs".* && mkdir -p "$run" "$gocache"
git -C "$repo" archive "$parent" | tar -x -C "$run"

spec=$(git -C "$repo" diff --name-only --diff-filter=AM "$parent" "$commit" | grep -E '(_test\.go$|(^|/)testdata/)' || true)
[[ -n "$spec" ]] && git -C "$repo" archive "$commit" -- $spec | tar -x -C "$run"
git -C "$run" init -q && git -C "$run" add -A
git -C "$run" -c user.email=e3@local -c user.name=e3 commit -qm "task start"

goroot=$(go env GOROOT)
modcache=$(go env GOMODCACHE)
tests=$(printf '%s\n' $spec | grep '_test\.go$' || true)
message=$(git -C "$repo" log -1 --format=%B "$commit")
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
tools=(Bash "Edit(./**)" "Write(./**)" Task)
mcp='{"mcpServers":{}}'

profile="$logs.sb"
cat > "$profile" <<EOF
(version 1)
(allow default)
(deny file-read* file-write*
  (subpath "$HOME/.ssh") (subpath "$HOME/code") (subpath "$HOME/.claude/projects")
  (literal "$HOME/.claude.json") (subpath "$workdir"))
(allow file-read* file-write* (subpath "$run") (subpath "$gocache"))
EOF

case "$arm" in
  A|B) ;;
  C)
    # cc-delegate in the sandbox: the launcher sets Go up offline, then the
    # server and its workers run under the profile. run-mcp.sh reads the local
    # gateway key itself; that file is outside the denied paths.
    launcher="$logs.mcp.sh"
    cat > "$launcher" <<EOF
#!/usr/bin/env bash
export DELEGATE_LOG_PATH="$logs.dispatches.jsonl"
export GOCACHE="$gocache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local PATH="$goroot/bin:\$PATH"
exec sandbox-exec -f "$profile" "$HOME/AI/cc-delegate/bin/run-mcp.sh"
EOF
    chmod +x "$launcher"
    mcp=$(python3 -I -c 'import json,sys; print(json.dumps({"mcpServers":{"delegate-local":{"command":sys.argv[1]}}}))' "$launcher")
    tools+=("mcp__delegate-local__delegate_batch" "mcp__delegate-local__delegate_to_local_agent" "mcp__delegate-local__list_local_agents")
    prompt="$prompt

How to work: plan once, then split the change into 1-6 bundles. Send the bundles to workers with
mcp__delegate-local__delegate_batch: one task per bundle, each with agent_name \"flash-builder\", model
\"deepseek-v4-1-flash\" and workdir \"$run\". Give each bundle its goal,
the files in scope, constraints, the verify command, and what done means. Workers never certify their own
work: after they finish, run the verify commands yourself and review the diff. Write code yourself only to
fix small problems the workers leave."
    ;;
  *) echo "unknown arm $arm" >&2; exit 2 ;;
esac
envs=(GOCACHE="$gocache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local "PATH=$goroot/bin:$PATH")
[[ "$arm" == B ]] && envs+=(CLAUDE_CODE_SUBAGENT_MODEL=claude-sonnet-5-5 CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1 ANTHROPIC_DEFAULT_HAIKU_MODEL=claude-haiku-5-5)

start=$(date +%s)
set +e
(cd "$run" && env "${envs[@]}" "$here/group_run.sh" "$limit_s" \
  claude -p "$prompt" --model claude-opus-5-5 --output-format stream-json --verbose \
  --max-turns 80 --permission-mode acceptEdits --allowedTools "${tools[@]}" \
  --no-session-persistence --setting-sources project --settings "$settings" \
  --mcp-config "$mcp" --strict-mcp-config \
  > "$logs.stream.jsonl" 2> "$logs.stderr")
exit_code=$?
set -e
secs=$(( $(date +%s) - start ))
git -C "$run" add -A >/dev/null 2>&1 && git -C "$run" diff --cached --stat > "$logs.diffstat" 2>/dev/null || true

# Grade: reset the whole test suite to the commit, then build and test in the sandbox.
find "$run" -name '*_test.go' -not -path '*/.git/*' -delete
find "$run" -type d -name testdata -not -path '*/.git/*' -prune -exec rm -rf {} +
suite_files=$(git -C "$repo" ls-tree -r --name-only "$commit" | grep -E '(_test\.go$|(^|/)testdata/)' || true)
[[ -n "$suite_files" ]] && git -C "$repo" archive "$commit" -- $suite_files | tar -x -C "$run"
gate_pkgs="$pkgs $(tr ',' ' ' <<<"$regress")"
accepted=0
(cd "$run" && sandbox-exec -f "$profile" env -i "PATH=$goroot/bin:/usr/bin:/bin" "HOME=$run" "GOCACHE=$gocache" \
  "GOMODCACHE=$modcache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local CGO_ENABLED=0 \
  sh -c "go build ./... && go test -count=1 $gate_pkgs" > "$logs.grade.log" 2>&1) && accepted=1

python3 -I "$here/parse_run.py" "$logs.stream.jsonl" "$id" "$arm" "$accepted" "$secs" "$exit_code" "$run" "$logs.dispatches.jsonl" >> "$results"
rm -rf "$gocache"
echo "$id $arm accepted=$accepted secs=$secs exit=$exit_code"
