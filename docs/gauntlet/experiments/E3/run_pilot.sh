#!/usr/bin/env bash
# Run the E3 pilot: every valid task in every arm. Arms of a task run one after
# another in a random order (no shared CPU, no sibling run to read, no fixed
# order that lets one arm warm the prompt cache for the next).
#
# Usage: run_pilot.sh <workdir> <results.csv> [first-task-id]
#   Run it from a copy of this directory (see run_arm.sh).
#
# Quota guard (D7), fails closed:
#  - After each task, read the highest 5-hour and 7-day plan utilization seen in
#    that task's streams. No signal at all stops the pilot.
#  - 7-day >= E3_MAX_7D (0.60): stop, leaving plan capacity for the user's work.
#  - 5-hour >= E3_MAX_5H (0.85): sleep until the 5-hour window resets.
#  - A run that hit a plan limit is not a quality failure: the task's rows are
#    removed and the task runs again after the reset.
set -euo pipefail

workdir=$1 results=$2 from=${3:-}
max5h=${E3_MAX_5H:-0.85} max7d=${E3_MAX_7D:-0.60}
here=$(cd "$(dirname "$0")" && pwd)
[[ -s "$results" ]] || python3 -I "$here/parse_run.py" --header > "$results"
trap 'kill $(jobs -p) 2>/dev/null || true' EXIT INT TERM

ids=$(awk -F'\t' '$6 == "valid" {print $1}' "$workdir/suite.tsv")
if [[ -n "$from" ]] && ! grep -qx "$from" <<<"$ids"; then
  echo "unknown task id $from" >&2; exit 2
fi

# guard <task>: prints "action wait_seconds" with action one of ok, sleep, stop, nosignal.
guard() {
  python3 -I - "$max5h" "$max7d" "$workdir"/logs/"$1"-*.stream.jsonl <<'PY'
import json, sys, time
max5h, max7d, paths = float(sys.argv[1]), float(sys.argv[2]), sys.argv[3:]
u5 = u7 = None
reset = 0
for path in paths:
    try:
        for line in open(path, errors="replace"):
            if '"rate_limit_event"' not in line:
                continue
            try:
                w = json.loads(line)["rate_limit_info"]["unifiedWindows"]
                f, s = w["five_hour"], w["seven_day"]
            except (ValueError, KeyError, TypeError):
                continue
            if isinstance(f.get("utilization"), (int, float)):
                u5 = max(u5 or 0, f["utilization"])
                r = f.get("resetsAt") or 0
                reset = max(reset, r / 1000 if r > 1e12 else r)  # accept seconds or milliseconds
            if isinstance(s.get("utilization"), (int, float)):
                u7 = max(u7 or 0, s["utilization"])
    except OSError:
        pass
if u5 is None or u7 is None:
    print("nosignal 0")
elif u7 >= max7d:
    print(f"stop 0 5h={u5} 7d={u7}")
elif u5 >= max5h:
    print(f"sleep {max(int(reset - time.time()) + 60, 60)} 5h={u5} 7d={u7}")
else:
    print(f"ok 0 5h={u5} 7d={u7}")
PY
}

started=0
[[ -z "$from" ]] && started=1
for id in $ids; do
  [[ "$id" == "$from" ]] && started=1
  (( started )) || continue
  while :; do
    for arm in $(printf 'A\nB\nC\n' | python3 -I -c 'import random,sys; l=sys.stdin.read().split(); random.shuffle(l); print(*l)'); do
      "$here/run_arm.sh" "$workdir" "$id" "$arm" "$results"
    done
    if awk -F, -v id="$id" '$1 == id && $18 == 1 {found=1} END {exit !found}' "$results"; then
      echo "$id hit a plan limit; removing its rows to rerun it after the reset"
      awk -F, -v id="$id" '$1 != id' "$results" > "$results.tmp" && mv "$results.tmp" "$results"
      sleep "${E3_LIMIT_BACKOFF_S:-1800}"
      continue
    fi
    break
  done
  read -r action wait_s rest <<<"$(guard "$id")"
  echo "$id done; plan $rest"
  # Keep a durable copy (aggregates only) so a crash cannot lose finished tasks.
  [[ -n "${E3_COPY_TO:-}" ]] && cp "$results" "$E3_COPY_TO"
  case "$action" in
    nosignal) echo "STOP: no plan utilization signal; cannot guard the quota. Resume: $0 $workdir $results <next id>"; exit 3 ;;
    stop) echo "STOP: 7-day plan utilization at or above $max7d. Resume later: $0 $workdir $results <next id>"; exit 3 ;;
    sleep) echo "5-hour window high; sleeping ${wait_s}s"; sleep "$wait_s" ;;
  esac
done
echo "pilot complete"
