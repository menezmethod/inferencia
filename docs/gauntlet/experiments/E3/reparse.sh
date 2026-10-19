#!/usr/bin/env bash
# Re-derive every results row from the saved streams with the current parse_run.py,
# keeping each row's measured accepted, secs and exit values.
# Usage: reparse.sh <workdir> <results.csv> > <new.csv>
set -euo pipefail
workdir=$1 results=$2
here=$(cd "$(dirname "$0")" && pwd)
python3 -I "$here/parse_run.py" --header
tail -n +2 "$results" | while IFS=, read -r task arm accepted secs code _; do
  python3 -I "$here/parse_run.py" "$workdir/logs/$task-$arm.stream.jsonl" "$task" "$arm" "$accepted" "$secs" "$code" \
    "$workdir/runs/$task-$arm" "$workdir/logs/$task-$arm.d/dispatches.jsonl"
done
