#!/usr/bin/env bash
# Run the E3 pilot: every valid task in every arm, one task (3 arms in parallel) at a time.
#
# Usage: run_pilot.sh <workdir> <results.csv> [first-task-id]
#
# Quota guard: Claude Code's stream reports the plan's 5-hour and 7-day
# utilization (rate_limit_event). After each task the driver reads the latest
# value. Above E3_MAX_5H (default 0.85) it sleeps until the 5-hour window
# resets; above E3_MAX_7D (default 0.60) it stops, so the experiment never
# uses the plan capacity the user needs for their own work. Re-run with the
# next task id to resume.
set -euo pipefail

workdir=$1 results=$2 from=${3:-T01}
max5h=${E3_MAX_5H:-0.85} max7d=${E3_MAX_7D:-0.60}
here=$(cd "$(dirname "$0")" && pwd)
[[ -s "$results" ]] || echo "task,arm,accepted,secs,exit,turns,cost_usd,input,cache_read,cache_write,output,worker_input,worker_cache_read,worker_output,result" > "$results"

utilization() { # prints: five_hour seven_day resets_at, from the newest stream of a task
  python3 -I - "$@" <<'PY'
import json, sys
best = None
for path in sys.argv[1:]:
    try:
        for line in open(path, errors="replace"):
            if '"rate_limit_event"' in line:
                info = json.loads(line)["rate_limit_info"]["unifiedWindows"]
                best = (info["five_hour"]["utilization"], info["seven_day"]["utilization"], info["five_hour"]["resetsAt"])
    except (OSError, ValueError, KeyError):
        pass
print(*(best or (0, 0, 0)))
PY
}

started=0
for id in $(awk -F'\t' '$6 == "valid" {print $1}' "$workdir/suite.tsv"); do
  [[ "$id" == "$from" ]] && started=1
  (( started )) || continue
  for arm in A B C; do "$here/run_arm.sh" "$workdir" "$id" "$arm" "$results" & done
  wait
  read -r u5 u7 reset <<<"$(utilization "$workdir"/runs/"$id"-*.stream.jsonl)"
  echo "$id done; plan utilization 5h=$u5 7d=$u7"
  if python3 -c "import sys; sys.exit(0 if $u7 >= $max7d else 1)"; then
    echo "STOP: 7-day plan utilization $u7 >= $max7d. Resume later with: $0 $workdir $results <next task id>"
    exit 3
  fi
  if python3 -c "import sys; sys.exit(0 if $u5 >= $max5h else 1)"; then
    wait_s=$(( reset - $(date +%s) + 60 ))
    (( wait_s > 0 )) && { echo "5-hour window at $u5; sleeping ${wait_s}s until it resets"; sleep "$wait_s"; }
  fi
done
echo "pilot complete"
