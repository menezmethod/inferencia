"""Analyze E3 results: cost per accepted task, paired bootstrap intervals, power.

Usage: python3 -I analyze.py results.csv [--seed 1] [--boot 10000]

Cost of a run = Claude's API-equivalent cost (total_cost_usd from the official
client, all its models included; summed from tokens when the run timed out,
flagged partial) + worker tokens at the catalog price of the model that served
them (arm C). Arm A is the baseline. Every task runs in every arm (paired).

Quality: non-inferiority at a 10-point margin, one-sided alpha 0.05, so the
verdict uses the 5th percentile of the paired accept-rate difference, the same
alpha as the sample-size formula n = (z_a + z_b)^2 * psi / (delta - d)^2.
"""
import csv
import math
import random
import sys
from collections import defaultdict

# USD per million tokens: (fresh input, cache read, output). internal/catalog/catalog.yaml, 2026-10-17.
WORKER_PRICES = {
    "deepseek-v4-1-flash": (0.15, 0.003, 0.60), "deepseek/deepseek-v4.1-flash": (0.15, 0.003, 0.60),
    "deepseek-v4.1-flash": (0.15, 0.003, 0.60),
}
Z_ALPHA, Z_BETA, MARGIN = 1.645, 0.842, 0.10


def worker_cost(r, unpriced):
    wi, wc, wo = (int(r[k] or 0) for k in ("worker_input", "worker_cache_read", "worker_output"))
    if not (wi or wc or wo):
        return 0.0
    models = [m for m in (r.get("worker_models") or "deepseek-v4-1-flash").split("|") if m]
    price = WORKER_PRICES.get(models[0]) if len(models) == 1 else None
    if price is None:
        unpriced.add(r.get("worker_models"))
        price = WORKER_PRICES["deepseek-v4-1-flash"]  # flagged below
    return (wi * price[0] + wc * price[1] + wo * price[2]) / 1e6


def pct(sorted_vals, q):
    if not sorted_vals:
        return float("nan")
    return sorted_vals[min(int(q * len(sorted_vals)), len(sorted_vals) - 1)]


def main():
    path = sys.argv[1]
    seed = int(sys.argv[sys.argv.index("--seed") + 1]) if "--seed" in sys.argv else 1
    boot = int(sys.argv[sys.argv.index("--boot") + 1]) if "--boot" in sys.argv else 10000
    runs, flags = defaultdict(dict), defaultdict(list)
    unpriced_workers = set()
    for r in csv.DictReader(open(path)):
        t, a = r["task"], r["arm"]
        if a in runs[t]:
            flags["duplicate rows (last kept)"].append(f"{t}-{a}")
        if r["cost_usd"] == "":
            flags["no cost at all (excluded)"].append(f"{t}-{a}")
            continue
        if int(r.get("outside_paths") or 0):
            flags["leak check fired (task excluded pending review)"].append(f"{t}-{a}")
        if int(r.get("cost_partial") or 0):
            flags["partial cost (timeout or crash; a lower bound)"].append(f"{t}-{a}")
        cost = float(r["cost_usd"]) + worker_cost(r, unpriced_workers)
        runs[t][a] = {"acc": int(r["accepted"]), "cost": cost, "secs": int(r["secs"] or 0),
                      "models": r.get("models", ""), "leak": int(r.get("outside_paths") or 0)}
    arms = sorted({a for v in runs.values() for a in v})
    tasks = sorted(t for t, v in runs.items() if all(a in v for a in arms) and not any(v[a]["leak"] for a in arms))
    n = len(tasks)
    print(f"tasks with all arms and no leak flag: {n}; arms: {', '.join(arms)}")
    for k, v in flags.items():
        print(f"  {k}: {len(v)} {' '.join(v[:12])}")
    if unpriced_workers:
        print(f"  worker models not in the price table (priced as deepseek-v4-1-flash): {unpriced_workers}")
    if n == 0 or "A" not in arms:
        print("nothing to compare")
        return

    print("\narm  accepted  rate   total_$  $/accepted  median_min")
    stats = {}
    for a in arms:
        acc = sum(runs[t][a]["acc"] for t in tasks)
        cost = sum(runs[t][a]["cost"] for t in tasks)
        mins = sorted(runs[t][a]["secs"] / 60 for t in tasks)
        stats[a] = (acc, cost)
        per = cost / acc if acc else float("inf")
        print(f"{a:>3}  {acc:>8}  {acc / n:.2f}  {cost:>7.2f}  {per:>10.2f}  {mins[n // 2]:>10.1f}")
    if "B" in arms:
        used = sum("sonnet" in runs[t]["B"]["models"] for t in tasks)
        print(f"\narm B manipulation check: {used} of {n} B runs used the configured subagent model")

    rng = random.Random(seed)
    print("\nvs A, paired bootstrap")
    for a in arms:
        if a == "A":
            continue
        ratios, diffs = [], []
        for _ in range(boot):
            s = [tasks[rng.randrange(n)] for _ in range(n)]
            acc_a, acc_x = sum(runs[t]["A"]["acc"] for t in s), sum(runs[t][a]["acc"] for t in s)
            cost_a, cost_x = sum(runs[t]["A"]["cost"] for t in s), sum(runs[t][a]["cost"] for t in s)
            if acc_a == 0:
                ratios.append(float("nan"))  # baseline accepted nothing: ratio undefined
            elif acc_x == 0:
                ratios.append(float("inf"))
            else:
                ratios.append((cost_x / acc_x) / (cost_a / acc_a))
            diffs.append((acc_x - acc_a) / n)
        undefined = sum(math.isnan(x) for x in ratios)
        ratios = sorted(x for x in ratios if not math.isnan(x))
        diffs.sort()
        acc_a, cost_a = stats["A"]
        acc_x, cost_x = stats[a]
        point = (cost_x / acc_x) / (cost_a / acc_a) if acc_a and acc_x else float("nan")
        d = (acc_x - acc_a) / n
        disc = sum(runs[t]["A"]["acc"] != runs[t][a]["acc"] for t in tasks) / n
        need = (Z_ALPHA + Z_BETA) ** 2 * max(disc, 1 / n) / max(MARGIN + d, 1e-9) ** 2 if MARGIN + d > 0 else float("inf")
        lo_d = pct(diffs, 0.05)
        print(f"  {a}: $/accepted ratio {point:.2f}, 95% CI [{pct(ratios, 0.025):.2f}, {pct(ratios, 0.975):.2f}], "
              f"one-sided 95% upper bound {pct(ratios, 0.95):.2f}" + (f" ({undefined} resamples undefined)" if undefined else ""))
        print(f"     accept-rate difference {d:+.2f}, one-sided 95% lower bound {lo_d:+.2f}; discordance {disc:.2f}; "
              f"paired n for 10-point non-inferiority: {need:.0f}")
        if lo_d > -MARGIN:
            print("     quality: non-inferior at a 10-point margin")
        else:
            print(f"     quality: underpowered: n={n}, rules out only a drop larger than {-lo_d * 100:.0f} points")
    print("\nTasks come from 2 repos, so a repo-cluster bootstrap is not informative; intervals are per task.")


if __name__ == "__main__":
    main()
