#!/usr/bin/env bash
# Build and validate the E3 task suite.
#
# A task is one commit that added a feature together with its tests. The task
# is to rebuild the feature from the commit message, starting at the parent
# commit. The commit's test files are the hidden tests. A candidate is valid
# only if the hidden tests FAIL at the parent and PASS at the commit.
#
# Usage: build_suite.sh <workdir> <candidates.tsv>
#   candidates.tsv columns: repo_path <TAB> commit
# Output: <workdir>/suite.tsv with: id repo commit parent packages verdict
#
# Sandbox (hard rule 10): each repo is cloned into <workdir> (the source repo is
# never modified), each check runs in a throwaway worktree of that clone, and
# HOME and GOCACHE point at temp dirs, so no home-directory credentials are
# reachable. Dependencies come from the Go module proxy and must match go.sum
# (-mod=readonly), the same check a normal `go test` makes.
set -euo pipefail

workdir=$1
candidates=$2
mkdir -p "$workdir"
out="$workdir/suite.tsv"
printf 'id\trepo\tcommit\tparent\tpackages\tverdict\n' > "$out"

modcache=$(go env GOMODCACHE)
gobin="$(go env GOROOT)/bin/go" # the toolchain go.mod selects; a bare PATH may find an older go
sandbox_env=(env -i "PATH=$PATH" "HOME=$workdir/home" "GOCACHE=$workdir/gocache"
  "GOMODCACHE=$modcache" "GOFLAGS=-mod=readonly" "GOPROXY=https://proxy.golang.org" "GOTOOLCHAIN=local" "CGO_ENABLED=0")
mkdir -p "$workdir/home" "$workdir/gocache"

run_tests() { # dir pkgs...
  local dir=$1; shift
  (cd "$dir" && "${sandbox_env[@]}" "$gobin" test -count=1 "$@" >"$dir.log" 2>&1)
}

n=0
while IFS=$'\t' read -r repo commit; do
  [[ -z "$repo" || "$repo" == \#* ]] && continue
  n=$((n + 1))
  id=$(printf 'T%02d' "$n")
  clone="$workdir/repos/$(basename "$repo")"
  [[ -d "$clone" ]] || git clone --quiet --no-local "$repo" "$clone"
  repo=$clone
  parent=$(git -C "$repo" rev-parse "$commit^")
  tests=$(git -C "$repo" diff --name-only --diff-filter=AM "$parent" "$commit" -- '*_test.go' | tr '\n' ' ')
  pkgs=$(for f in $tests; do echo "./$(dirname "$f")"; done | sort -u | tr '\n' ' ')
  wt="$workdir/wt-$id"
  git -C "$repo" worktree add --detach --force "$wt" "$parent" >/dev/null 2>&1

  verdict=valid
  # At the commit, the hidden tests must pass.
  git -C "$wt" checkout --quiet "$commit"
  run_tests "$wt" $pkgs || verdict=invalid-fails-at-commit
  # At the parent plus the hidden tests, they must fail.
  if [[ $verdict == valid ]]; then
    git -C "$wt" checkout --quiet "$parent"
    git -C "$wt" checkout --quiet "$commit" -- $tests
    if run_tests "$wt" $pkgs; then verdict=invalid-passes-at-parent; fi
  fi
  git -C "$repo" worktree remove --force "$wt" >/dev/null 2>&1 || true
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$id" "$repo" "$commit" "$parent" "$pkgs" "$verdict" >> "$out"
  echo "$id $(basename "$repo") $commit $verdict"
done < "$candidates"
