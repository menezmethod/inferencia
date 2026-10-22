"""Copy E3 arm A (solo claude-opus-5-5) rows into matrix.csv format.

Usage: python3 -I import_pilot.py <pilot.csv> >> matrix.csv

Arm A is the matrix's Opus column: same suite, same prompt, same harness, same
grading (run 2026-10-17). visible_pass was not recorded then, so it is empty. That
is safe: Opus is the last step of every cascade, so no policy escalates from it.
"""
import csv
import sys

sys.path.insert(0, __file__.rsplit("/", 1)[0])
from parse_cell import FIELDS  # noqa: E402


def convert(rows):
    out = []
    for r in rows:
        if r["arm"] != "A":
            continue
        out.append({
            "task": r["task"], "model": "claude-opus-5-5", "accepted": r["accepted"], "visible_pass": "",
            "secs": r["secs"], "exit": r["exit"], "turns": r["turns"], "cost_usd": r["cost_usd"],
            "cost_tokens": "", "cost_source": "client-e3" if r["cost_partial"] == "0" else "messages-e3",
            "input": r["input"], "cache_read": r["cache_read"], "cache_write": r["cache_write"], "output": r["output"],
            "models": r["models"], "quota_hit": r["quota_hit"], "outside_paths": r["outside_paths"], "result": r["result"],
        })
    return out


def main():
    w = csv.DictWriter(sys.stdout, FIELDS, lineterminator="\n")
    for row in convert(csv.DictReader(open(sys.argv[1]))):
        w.writerow(row)


if __name__ == "__main__":
    main()
