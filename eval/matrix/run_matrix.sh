#!/usr/bin/env bash
# Run every missing matrix cell: each valid task on each model, solo.
#
# Usage: run_matrix.sh <workdir> <matrix.csv>
#   Run it from a copy of this directory (see run_cell.sh). It is safe to stop and
#   rerun: cells already in matrix.csv are skipped.
#   EVAL_MODELS overrides the model list (space separated).
#
# Guards (fail closed):
#  - Gateway: checked before the first gateway cell; down = stop.
#  - Claude plan: after each task, the highest 7-day and 5-hour utilization seen in
#    that task's Claude streams. 7-day >= EVAL_MAX_7D (0.60) = stop; 5-hour >=
#    EVAL_MAX_5H (0.85) = sleep until reset. A task that ran Claude cells but gave
#    no signal = stop.
#  - Command Code: total cost of gateway cells in matrix.csv >= EVAL_MAX_CC_USD
#    (10) = stop before the next gateway cell.
#  EVAL_PILOT_CSV: on a new matrix.csv, import E3 arm A from this file (the Opus column).
#  - A cell that hit a plan limit is removed and rerun after a backoff.
set -euo pipefail

workdir=$1 results=$2
here=$(cd "$(dirname "$0")" && pwd)
models=${EVAL_MODELS:-"claude-sonnet-5-5 claude-haiku-5-5 deepseek-v4-1-flash muse-spark-1.3-contributor"}
max5h=${EVAL_MAX_5H:-0.85} max7d=${EVAL_MAX_7D:-0.60} max_cc=${EVAL_MAX_CC_USD:-10}
gateway=${EVAL_GATEWAY_URL:-http://127.0.0.1:4000}
pilot=${EVAL_PILOT_CSV:-}  # E3 pilot.csv: its arm A rows are the Opus column
trap 'kill $(jobs -p) 2>/dev/null || true' EXIT INT TERM

if [[ ! -s "$results" ]]; then
  python3 -I "$here/parse_cell.py" --header > "$results"
  if [[ -n "$pilot" ]]; then
    python3 -I "$here/import_pilot.py" "$pilot" >> "$results"
    echo "imported the Opus column from $pilot"
  fi
fi

have() { awk -F, -v t="$1" -v m="$2" '$1 == t && $2 == m {f=1} END {exit !f}' "$results"; }
cc_spend() { awk -F, 'NR > 1 && $2 !~ /^claude-/ && $8 != "" {s += $8} END {printf "%.4f", s + 0}' "$results"; }
gateway_ok() { curl -fsS -m 5 "$gateway/health/liveliness" >/dev/null 2>&1; }

claude_guard() { # task: prints "action wait_seconds detail"
  python3 -I - "$max5h" "$max7d" "$workdir"/logs/"$1"-claude-*.stream.jsonl <<'PY'
import json, sys, time
max5h, max7d, paths = float(sys.argv[1]), float(sys.argv[2]), [p for p in sys.argv[3:] if "*" not in p]
if not paths:
    print("none 0"); sys.exit()
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
                reset = max(reset, r / 1000 if r > 1e12 else r)
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

ids=$(awk -F'\t' '$6 == "valid" {print $1}' "$workdir/suite.tsv")
gateway_checked=0
for id in $ids; do
  ran_claude=0
  for model in $(printf '%s\n' $models | python3 -I -c 'import random,sys; l=sys.stdin.read().split(); random.shuffle(l); print(*l)'); do
    have "$id" "$model" && continue
    if [[ $model != claude-* ]]; then
      if (( ! gateway_checked )); then
        gateway_ok || { echo "STOP: gateway $gateway is down. Start ~/AI/cc-delegate/bin/start-gateway.sh, then rerun."; exit 3; }
        gateway_checked=1
      fi
      spend=$(cc_spend)
      if python3 -I -c "import sys; sys.exit(0 if float(sys.argv[1]) >= float(sys.argv[2]) else 1)" "$spend" "$max_cc"; then
        echo "STOP: Command Code spend \$$spend reached the \$$max_cc limit."; exit 3
      fi
    fi
    set +e
    "$here/run_cell.sh" "$workdir" "$id" "$model" "$results"
    rc=$?
    set -e
    (( rc == 4 )) && continue   # not allowed for this task (data rule)
    (( rc != 0 )) && { echo "STOP: run_cell failed ($rc) on $id $model"; exit 3; }
    [[ $model == claude-* ]] && ran_claude=1
    if awk -F, -v t="$id" -v m="$model" '$1 == t && $2 == m && $16 == 1 {f=1} END {exit !f}' "$results"; then
      echo "$id $model hit a plan limit; removing the row, then rerunning after a backoff"
      awk -F, -v t="$id" -v m="$model" '!($1 == t && $2 == m)' "$results" > "$results.tmp" && mv "$results.tmp" "$results"
      sleep "${EVAL_LIMIT_BACKOFF_S:-1800}"
      exec "$0" "$@"
    fi
  done
  [[ -n "${EVAL_COPY_TO:-}" ]] && cp "$results" "$EVAL_COPY_TO"
  (( ran_claude )) || continue
  read -r action wait_s rest <<<"$(claude_guard "$id")"
  echo "$id done; plan $rest; Command Code \$$(cc_spend)"
  case "$action" in
    nosignal) echo "STOP: no plan utilization signal; cannot guard the quota. Rerun the same command later."; exit 3 ;;
    stop) echo "STOP: 7-day plan utilization at or above $max7d. Rerun the same command later."; exit 3 ;;
    sleep) echo "5-hour window high; sleeping ${wait_s}s"; sleep "$wait_s" ;;
  esac
done
echo "matrix complete"
