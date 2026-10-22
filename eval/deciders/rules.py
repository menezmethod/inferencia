"""Frozen rules decider (eval/README.md section 5). Do not tune after the first matrix run.

Usage: python3 -I rules.py < features.jsonl > decisions-rules.csv
"""
import csv
import json
import sys
import time

FIELDS = ["decider", "task", "start_tier", "p_fast", "tier_choice", "latency_ms", "cost_usd"]


def tier(f):
    if f["packages"] >= 3 or f["test_lines"] >= 800:
        return "frontier"
    if f["packages"] >= 2 or f["test_lines"] >= 300:
        return "strong"
    return "fast"


def main():
    w = csv.DictWriter(sys.stdout, FIELDS, lineterminator="\n")
    w.writeheader()
    for line in sys.stdin:
        if not line.strip():
            continue
        f = json.loads(line)
        t0 = time.perf_counter()
        t = tier(f)
        ms = (time.perf_counter() - t0) * 1000
        w.writerow({"decider": "rules", "task": f["task"], "start_tier": t, "p_fast": "", "tier_choice": "",
                    "latency_ms": f"{ms:.3f}", "cost_usd": 0})


if __name__ == "__main__":
    main()
