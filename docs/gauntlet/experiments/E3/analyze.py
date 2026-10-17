"""Analyze E3 results: cost per accepted task, paired bootstrap CIs, power.

Usage: python3 -I analyze.py results.csv [--seed 1] [--boot 10000]

Cost of a run = Claude's API-equivalent cost (total_cost_usd from the official
client, which includes subagent and background models) + worker tokens priced
at the catalog list price of the cc-delegate worker model (arm C only).
Arm A is the baseline. Every task ran in every arm (paired design).
"""
import csv
import random
import sys
from collections import defaultdict

# deepseek-v4.1-flash, USD per million tokens (internal/catalog/catalog.yaml, OpenRouter, 2026-10-17)
WORKER = {"input": 0.15, "cache_read": 0.003, "output": 0.60}
Z_ALPHA, Z_BETA, MARGIN = 1.645, 0.842, 0.10  # one-sided alpha 0.05, power 0.8, 10 points


def run_cost(r):
    claude = float(r["cost_usd"] or 0)
    wi, wc, wo = (int(r[k] or 0) for k in ("worker_input", "worker_cache_read", "worker_output"))
    worker = ((wi - wc) * WORKER["input"] + wc * WORKER["cache_read"] + wo * WORKER["output"]) / 1e6
    return claude + worker, worker


def main():
    path = sys.argv[1]
    seed = int(sys.argv[sys.argv.index("--seed") + 1]) if "--seed" in sys.argv else 1
    boot = int(sys.argv[sys.argv.index("--boot") + 1]) if "--boot" in sys.argv else 10000
    runs = defaultdict(dict)  # task -> arm -> (accepted, cost, worker_cost, secs)
    for r in csv.DictReader(open(path)):
        cost, worker = run_cost(r)
        runs[r["task"]][r["arm"]] = (int(r["accepted"]), cost, worker, int(r["secs"]))
    arms = sorted({a for t in runs.values() for a in t})
    tasks = sorted(t for t, v in runs.items() if all(a in v for a in arms))
    n = len(tasks)
    print(f"tasks with all arms: {n}; arms: {', '.join(arms)}\n")

    print("arm  accepted  accept_rate  total_$  $/accepted  worker_$  median_min")
    stats = {}
    for a in arms:
        acc = sum(runs[t][a][0] for t in tasks)
        cost = sum(runs[t][a][1] for t in tasks)
        worker = sum(runs[t][a][2] for t in tasks)
        mins = sorted(runs[t][a][3] / 60 for t in tasks)
        per = cost / acc if acc else float("inf")
        stats[a] = (acc, cost, per)
        print(f"{a:>3}  {acc:>8}  {acc / n:>11.2f}  {cost:>7.2f}  {per:>10.2f}  {worker:>8.2f}  {mins[n // 2] if n else 0:>10.1f}")

    rng = random.Random(seed)
    print("\nvs A (paired bootstrap, 95% percentile intervals)")
    print("arm  $/acc ratio [95% CI]        accept diff [95% CI]   discordance  n for 10pt NI")
    for a in arms:
        if a == "A":
            continue
        ratios, diffs = [], []
        for _ in range(boot):
            sample = [tasks[rng.randrange(n)] for _ in range(n)]
            acc_a = sum(runs[t]["A"][0] for t in sample)
            acc_x = sum(runs[t][a][0] for t in sample)
            cost_a = sum(runs[t]["A"][1] for t in sample)
            cost_x = sum(runs[t][a][1] for t in sample)
            if acc_a and acc_x:
                ratios.append((cost_x / acc_x) / (cost_a / acc_a))
            diffs.append((acc_x - acc_a) / n)
        ratios.sort()
        diffs.sort()
        lo_r, hi_r = (ratios[int(0.025 * len(ratios))], ratios[int(0.975 * len(ratios)) - 1]) if ratios else (float("nan"),) * 2
        lo_d, hi_d = diffs[int(0.025 * boot)], diffs[int(0.975 * boot) - 1]
        point = stats[a][2] / stats["A"][2] if stats["A"][2] not in (0, float("inf")) else float("nan")
        disc = sum(runs[t]["A"][0] != runs[t][a][0] for t in tasks) / n
        need = (Z_ALPHA + Z_BETA) ** 2 * max(disc, 1 / n) / MARGIN ** 2
        print(f"{a:>3}  {point:.2f} [{lo_r:.2f}, {hi_r:.2f}]  {(stats[a][0] - stats['A'][0]) / n:+.2f} [{lo_d:+.2f}, {hi_d:+.2f}]"
              f"   {disc:>10.2f}  {need:>12.0f}")
        ruled_out = -lo_d if lo_d < 0 else 0
        verdict = "non-inferior at 10pt" if lo_d > -MARGIN else f"underpowered: n={n}, rules out only a drop larger than {ruled_out * 100:.0f} points"
        print(f"     quality: {verdict}")
    print("\nTasks come from 2 repos, so a repo-cluster bootstrap is not informative; intervals are per-task.")


if __name__ == "__main__":
    main()
