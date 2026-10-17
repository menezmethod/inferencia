#!/usr/bin/env bash
# Build and validate the E3 task suite.
#
# A task is one commit that added a feature together with its tests. The task
# is to rebuild the feature, starting at the parent commit, with the commit's
# test files as the spec (D6). A candidate is valid only if its tests FAIL at
# the parent (with the commit's tests and testdata in place) and PASS at the
# commit three runs in a row (to drop flaky tasks).
#
# Usage: build_suite.sh <workdir> <candidates.tsv>
#   candidates.tsv columns: repo_path <TAB> commit
# Output: <workdir>/suite.tsv with: id repo commit parent packages verdict regress
#   regress = import paths of every package whose tests pass at the commit; grading
#   requires them all to still pass, so an agent cannot break other packages.
#
# Sandbox (hard rule 10): each repo is cloned without a checkout into <workdir>
# (the source repo is never modified, and no working tree of later commits
# exists), each check runs in a throwaway worktree of that clone, and HOME and
# GOCACHE point at temp dirs. Dependencies come from the Go module proxy and must
# match go.sum (-mod=readonly), which also warms the module cache the arms use
# offline.
set -euo pipefail

workdir=$1
candidates=$2
mkdir -p "$workdir"
out="$workdir/suite.tsv"
printf 'id\trepo\tcommit\tparent\tpackages\tverdict\tregress\n' > "$out"

modcache=$(go env GOMODCACHE)
gobin="$(go env GOROOT)/bin/go" # the toolchain go.mod selects; a bare PATH may find an older go
sandbox_env=(env -i "PATH=$PATH" "HOME=$workdir/home" "GOCACHE=$workdir/gocache"
  "GOMODCACHE=$modcache" "GOFLAGS=-mod=readonly" "GOPROXY=https://proxy.golang.org" "GOTOOLCHAIN=local" "CGO_ENABLED=0")
mkdir -p "$workdir/home" "$workdir/gocache"

run_tests() { # dir pkgs...
  local dir=$1; shift
  (cd "$dir" && "${sandbox_env[@]}" "$gobin" test -count=1 "$@" >>"$dir.log" 2>&1)
}

# spec_files repo parent commit: the commit's added or changed test files and testdata.
spec_files() {
  git -C "$1" diff --name-only --diff-filter=AM "$2" "$3" | grep -E '(_test\.go$|(^|/)testdata/)' || true
}

n=0
while IFS=$'\t' read -r repo commit; do
  [[ -z "$repo" || "$repo" == \#* ]] && continue
  n=$((n + 1))
  id=$(printf 'T%02d' "$n")
  clone="$workdir/repos/$(basename "$repo")"
  [[ -d "$clone" ]] || git clone --quiet --no-local --no-checkout "$repo" "$clone"
  repo=$clone
  parent=$(git -C "$repo" rev-parse "$commit^")
  files=$(spec_files "$repo" "$parent" "$commit")
  pkgs=$(for f in $files; do [[ $f == *_test.go ]] && echo "./$(dirname "$f")"; done | sort -u | tr '\n' ' ')
  wt="$workdir/wt-$id"
  rm -f "$wt.log"
  git -C "$repo" worktree add --detach --force "$wt" "$commit" >/dev/null 2>&1

  verdict=valid
  regress=""
  # At the commit, the task's tests must pass three times in a row.
  for _ in 1 2 3; do
    run_tests "$wt" $pkgs || { verdict=invalid-fails-at-commit; break; }
  done
  if [[ $verdict == valid ]]; then
    # Every package that passes at the commit becomes the regression set.
    regress=$(cd "$wt" && "${sandbox_env[@]}" "$gobin" test -count=1 ./... 2>/dev/null | awk '$1 == "ok" {print $2}' | tr '\n' ',' | sed 's/,$//') || true
    # At the parent with the commit's tests and testdata in place, the tests must fail.
    git -C "$wt" checkout --quiet "$parent"
    git -C "$wt" checkout --quiet "$commit" -- $files
    if run_tests "$wt" $pkgs; then verdict=invalid-passes-at-parent; fi
  fi
  git -C "$repo" worktree remove --force "$wt" >/dev/null 2>&1 || true
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$id" "$repo" "$commit" "$parent" "$pkgs" "$verdict" "$regress" >> "$out"
  echo "$id $(basename "$repo") $commit $verdict"
done < "$candidates"
