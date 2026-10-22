"""Analyze the routing matrix against the frozen gates in eval/README.md.

Usage: python3 -I matrix.py <matrix.csv> <suite.tsv> [decisions-*.csv ...] [--seed 1] [--boot 10000]

Prints a Markdown report: policies, Pareto frontier, gate verdicts G1, G2, G3, G5,
and the threshold sweep (APGR). G4 (ledger vs bill) is measured separately.

Decision files have the columns: decider, task, start_tier, p_fast, tier_choice,
latency_ms, cost_usd. p_fast and tier_choice may be empty (rules has neither).
"""
import csv
import random
import sys
from collections import defaultdict

LADDER = ["deepseek-v4-1-flash", "claude-sonnet-5-5", "claude-opus-5-5"]  # fast, strong, frontier
TIER_INDEX = {"fast": 0, "strong": 1, "frontier": 2}
FRONTIER = LADDER[-1]
FIXED = ["claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5", "deepseek-v4-1-flash"]
PUBLIC_ONLY = ["muse-spark-1.3-contributor"]
MARGIN = 0.10


class DataError(Exception):
    pass


def load_matrix(path):
    cells, flags = defaultdict(dict), defaultdict(list)
    with open(path) as f:
        rows = list(csv.DictReader(f))
    for r in rows:
        t, m = r["task"], r["model"]
        if m in cells[t]:
            flags["duplicate rows (last kept)"].append(f"{t}/{m}")
        if r["cost_usd"] == "":
            raise DataError(f"{t}/{m} has no cost; a missing cost is never 0. Fix the price, then reparse.")
        if r.get("quota_hit") == "1" or r.get("invalid") == "1":
            raise DataError(f"{t}/{m} is not a valid run (API error or plan limit); rerun the cell before analysis.")
        check = r.get("cost_check") or ""
        if check and abs(float(check) - float(r["cost_usd"])) > 0.05 * max(float(r["cost_usd"]), 1e-9):
            flags["cost check differs by more than 5%"].append(f"{t}/{m}")
        cells[t][m] = {"acc": int(r["accepted"]), "vis": r["visible_pass"], "cost": float(r["cost_usd"]),
                       "leak": int(r.get("outside_paths") or 0)}
    return cells, flags


def load_suite(path):
    with open(path) as f:
        return {r["id"]: r["repo"].rstrip("/").rsplit("/", 1)[-1] for r in csv.DictReader(f, delimiter="\t")
                if r["verdict"] == "valid"}


def load_decisions(paths):
    out = defaultdict(dict)
    for p in paths:
        with open(p) as f:
            rows = list(csv.DictReader(f))
        for r in rows:
            if r.get("latency_ms") in (None, "") or r.get("cost_usd") in (None, ""):
                raise DataError(f"{p}: {r['task']} has no latency or cost; G3 cannot pass on a missing value")
            if r["start_tier"] not in TIER_INDEX:
                raise DataError(f"{p}: unknown tier {r['start_tier']!r} for {r['task']}")
            out[r["decider"]][r["task"]] = {
                "tier": r["start_tier"], "choice": r.get("tier_choice") or "",
                "p": float(r["p_fast"]) if r.get("p_fast") not in (None, "") else None,
                "ms": float(r["latency_ms"]), "cost": float(r["cost_usd"])}
    return out


def complete_tasks(cells, flags):
    """Tasks with every ladder model and Haiku, and no leak."""
    keep = []
    for t in sorted(cells):
        missing = [m for m in FIXED if m not in cells[t]]
        if missing:
            flags["incomplete (excluded)"].append(f"{t}: missing {','.join(missing)}")
        elif any(c["leak"] for c in cells[t].values()):
            flags["leak check fired (excluded pending review)"].append(t)
        else:
            keep.append(t)
    return keep


# ---- policies: each returns {task: (cost, accepted)} -------------------------------

def fixed(cells, tasks, model):
    return {t: (cells[t][model]["cost"], cells[t][model]["acc"]) for t in tasks if model in cells[t]}


def oracle(cells, tasks):
    out = {}
    for t in tasks:
        ok = [c["cost"] for c in cells[t].values() if c["acc"]]
        out[t] = (min(ok), 1) if ok else (cells[t][FRONTIER]["cost"], 0)
    return out


def random_policy(cells, tasks):
    out = {}
    for t in tasks:
        cs = list(cells[t].values())
        out[t] = (sum(c["cost"] for c in cs) / len(cs), sum(c["acc"] for c in cs) / len(cs))
    return out


def cascade(cells, tasks, start=None, signal="vis"):
    """Start at a ladder index (per task via dict, or 0), escalate when the signal fails."""
    out = {}
    for t in tasks:
        i = start.get(t, 0) if isinstance(start, dict) else (start or 0)
        cost = 0.0
        while True:
            c = cells[t][LADDER[i]]
            cost += c["cost"]
            if i == len(LADDER) - 1:
                out[t] = (cost, c["acc"])
                break
            if signal == "vis":
                if c["vis"] == "":
                    raise DataError(f"{t}/{LADDER[i]} has no visible_pass; the cascade cannot run")
                passed = c["vis"] == "1"
            else:
                passed = c["acc"] == 1
            if passed:
                out[t] = (cost, c["acc"])
                break
            i += 1
    return out


def decider_start(dec, threshold=None):
    """Ladder start per task. With a threshold, recompute from p_fast and tier_choice."""
    out = {}
    for t, d in dec.items():
        if threshold is None or d["p"] is None:
            out[t] = TIER_INDEX[d["tier"]]
        elif d["p"] >= threshold:
            out[t] = 0
        else:
            out[t] = max(TIER_INDEX.get(d["choice"], 1), 1)
    return out


# ---- statistics ----------------------------------------------------------------------

def cpa(pol, tasks):
    cost = sum(pol[t][0] for t in tasks)
    acc = sum(pol[t][1] for t in tasks)
    return cost / acc if acc else float("inf")


def rate(pol, tasks):
    return sum(pol[t][1] for t in tasks) / len(tasks)


def pct(vals, q):
    vals = sorted(vals)
    return vals[min(int(q * len(vals)), len(vals) - 1)]


def paired(pol, base, tasks, boot, rng):
    """Point and bootstrap bounds for the CPA ratio and the accept-rate difference."""
    ratios, diffs = [], []
    for _ in range(boot):
        s = [rng.choice(tasks) for _ in tasks]
        b = cpa(base, s)
        ratios.append(cpa(pol, s) / b if b not in (0, float("inf")) else float("inf"))
        diffs.append(rate(pol, s) - rate(base, s))
    return {"ratio": cpa(pol, tasks) / cpa(base, tasks), "lo": pct(ratios, 0.025), "hi": pct(ratios, 0.975),
            "ub": pct(ratios, 0.95), "diff": rate(pol, tasks) - rate(base, tasks), "diff_lb": pct(diffs, 0.05)}


def brier(ps, ys):
    return sum((p - y) ** 2 for p, y in zip(ps, ys)) / len(ps)


def ece(ps, ys, bins=5):
    total, rows = 0.0, []
    for b in range(bins):
        lo, hi = b / bins, (b + 1) / bins
        idx = [i for i, p in enumerate(ps) if lo <= p < hi or (b == bins - 1 and p == 1.0)]
        if not idx:
            continue
        conf = sum(ps[i] for i in idx) / len(idx)
        acc = sum(ys[i] for i in idx) / len(idx)
        total += len(idx) / len(ps) * abs(conf - acc)
        rows.append((f"{lo:.1f}-{hi:.1f}", len(idx), conf, acc))
    return total, rows


def pareto(points):
    """points: {name: (cpa_or_cost, accept_rate)} -> names not dominated."""
    keep = []
    for n, (c, a) in points.items():
        if not any((c2 <= c and a2 >= a) and (c2 < c or a2 > a) for n2, (c2, a2) in points.items() if n2 != n):
            keep.append(n)
    return sorted(keep, key=lambda n: points[n][0])


def apgr(curve):
    """Area under PGR vs cost share, both clamped to [0, 1], trapezoid, with (0,0) and (1,1)."""
    pts = sorted({(min(max(c, 0), 1), min(max(p, 0), 1)) for c, p in curve} | {(0.0, 0.0), (1.0, 1.0)})
    return sum((x2 - x1) * (y1 + y2) / 2 for (x1, y1), (x2, y2) in zip(pts, pts[1:]))


# ---- report --------------------------------------------------------------------------

def analyze(cells, suite, decisions, flags, boot=10000, seed=1):
    rng = random.Random(seed)
    tasks = [t for t in complete_tasks(cells, flags) if t in suite]
    if len(tasks) < 2:
        raise DataError(f"only {len(tasks)} complete tasks; run the matrix first")
    opus = fixed(cells, tasks, FRONTIER)
    pols = {f"always {m}": fixed(cells, tasks, m) for m in FIXED}
    pols["oracle (bound)"] = oracle(cells, tasks)
    pols["random"] = random_policy(cells, tasks)
    pols["cascade-visible"] = cascade(cells, tasks, 0, "vis")
    pols["cascade-hidden (bound)"] = cascade(cells, tasks, 0, "acc")
    for name, dec in decisions.items():
        missing = [t for t in tasks if t not in dec]
        if missing:
            raise DataError(f"decider {name} has no decision for {','.join(missing)}")
        pols[name] = cascade(cells, tasks, decider_start(dec), "vis")

    out = [f"# Routing matrix report\n", f"Tasks analyzed: {len(tasks)} (complete, no leak). "
           f"Bootstrap: {boot} resamples, seed {seed}.\n"]
    for k, v in flags.items():
        out.append(f"- **{k}:** {', '.join(v)}")
    out.append("\n## Policies (paired against always-Opus)\n")
    out.append("| Policy | Accept rate | Total $ | $ per accepted | Ratio vs Opus [95% CI] | Accept diff (one-sided 95% LB) | Quality floor |")
    out.append("| --- | --- | --- | --- | --- | --- | --- |")
    stats = {}
    for name, pol in pols.items():
        s = paired(pol, opus, tasks, boot, rng)
        s["floor"] = s["diff_lb"] > -MARGIN
        stats[name] = s
        out.append(f"| {name} | {rate(pol, tasks):.0%} | {sum(pol[t][0] for t in tasks):.2f} | {cpa(pol, tasks):.3f} | "
                   f"{s['ratio']:.2f} [{s['lo']:.2f}, {s['hi']:.2f}] | {s['diff']:+.2f} ({s['diff_lb']:+.2f}) | "
                   f"{'holds' if s['floor'] else 'FAILS'} |")

    public = [t for t in tasks if suite[t] == "inferencia" and all(m in cells[t] for m in PUBLIC_ONLY)]
    for m in PUBLIC_ONLY:
        if public:
            s = paired(fixed(cells, public, m), fixed(cells, public, FRONTIER), public, boot, rng)
            out.append(f"\nPublic-only model {m} on {len(public)} public tasks: ratio vs Opus {s['ratio']:.2f} "
                       f"[{s['lo']:.2f}, {s['hi']:.2f}], accept diff {s['diff']:+.2f} (LB {s['diff_lb']:+.2f}).")

    points = {n: (cpa(p, tasks), rate(p, tasks)) for n, p in pols.items() if "bound" not in n}
    out.append("\n## Pareto frontier (real policies)\n")
    out.append(", ".join(f"{n} ({points[n][0]:.3f} $/acc, {points[n][1]:.0%})" for n in pareto(points)))

    # G1: against the best fixed policy that holds the floor.
    holders = [f"always {m}" for m in FIXED if stats[f"always {m}"]["floor"]]
    best = min(holders, key=lambda n: cpa(pols[n], tasks))
    out.append(f"\n## Gates\n\nBest fixed policy that holds the quality floor: **{best}**.\n")
    out.append("| Gate | Subject | Value | Verdict |\n| --- | --- | --- | --- |")
    verdict = {}
    for name in ["cascade-visible"] + list(decisions):
        s = stats[name]
        if best == f"always {FRONTIER}":
            ub, need, vs = s["ub"], 0.9, "Opus"
        else:
            ub, need, vs = paired(pols[name], pols[best], tasks, boot, rng)["ub"], 1.0, best
        ok = s["floor"] and ub < need
        verdict[name] = ok
        out.append(f"| G1 | {name} | floor {'holds' if s['floor'] else 'fails'}; ratio UB vs {vs} {ub:.2f} (need < {need}) | "
                   f"{'PASS' if ok else 'FAIL'} |")

    ys = [cells[t][LADDER[0]]["acc"] for t in tasks]
    for name, dec in decisions.items():
        ps = [dec[t]["p"] for t in tasks]
        if None not in ps:
            base = sum(ys) / len(ys)
            b, b0 = brier(ps, ys), brier([base] * len(ys), ys)
            e, rows = ece(ps, ys)
            ok = b < b0 and e <= 0.10
            out.append(f"| G2 | {name} p_fast vs {LADDER[0]} | Brier {b:.3f} vs base rate {b0:.3f}; ECE {e:.3f} | {'PASS' if ok else 'FAIL'} |")
            verdict[name + " G2"] = ok
            verdict[name + " rows"] = rows
        lat = pct([dec[t]["ms"] for t in tasks], 0.95)
        dcost = sum(dec[t]["cost"] for t in tasks) / len(tasks)
        saving = (sum(opus[t][0] for t in tasks) - sum(pols[name][t][0] for t in tasks)) / len(tasks)
        ok = lat <= 1000 and saving > 0 and dcost <= 0.1 * saving
        out.append(f"| G3 | {name} | p95 {lat:.0f} ms; ${dcost:.5f}/decision vs ${saving:.4f} saving/task | {'PASS' if ok else 'FAIL'} |")

    if "laya" in decisions and "rules" in decisions:
        ub = paired(pols["laya"], pols["rules"], tasks, boot, rng)["ub"]
        ok = verdict.get("laya", False) and ub < 1.0
        out.append(f"| G5 | laya vs rules | ratio UB {ub:.2f} (need < 1.0) and laya G1 | {'PASS' if ok else 'FAIL'} |")

    for name in decisions:
        if verdict.get(name + " rows"):
            out.append(f"\n### Reliability: {name}\n\n| Bin | n | Mean p_fast | Observed accept |\n| --- | --- | --- | --- |")
            out += [f"| {b} | {n} | {c:.2f} | {a:.2f} |" for b, n, c, a in verdict[name + " rows"]]

    weak, strong = fixed(cells, tasks, LADDER[0]), opus
    gap_acc, gap_cost = rate(strong, tasks) - rate(weak, tasks), sum(strong[t][0] for t in tasks) - sum(weak[t][0] for t in tasks)
    out.append("\n## Threshold sweep (secondary result)\n")
    if gap_acc <= 0 or gap_cost <= 0:
        out.append(f"PGR is undefined: always-{LADDER[0]} is not worse and cheaper than Opus on this set.")
    else:
        for name, dec in decisions.items():
            if any(d["p"] is None for d in dec.values()):
                continue
            curve = []
            for k in range(21):
                pol = cascade(cells, tasks, decider_start(dec, k / 20), "vis")
                curve.append(((sum(pol[t][0] for t in tasks) - sum(weak[t][0] for t in tasks)) / gap_cost,
                              (rate(pol, tasks) - rate(weak, tasks)) / gap_acc))
            out.append(f"- {name}: APGR {apgr(curve):.2f} over 21 thresholds "
                       f"(area under PGR vs dollar cost share; see eval/README.md section 6).")
    return "\n".join(out) + "\n"


def main(argv):
    seed = int(argv[argv.index("--seed") + 1]) if "--seed" in argv else 1
    boot = int(argv[argv.index("--boot") + 1]) if "--boot" in argv else 10000
    files = [a for i, a in enumerate(argv[1:], 1) if not a.startswith("--") and argv[i - 1] not in ("--seed", "--boot")]
    cells, flags = load_matrix(files[0])
    sys.stdout.write(analyze(cells, load_suite(files[1]), load_decisions(files[2:]), flags, boot, seed))


if __name__ == "__main__":
    try:
        main(sys.argv)
    except DataError as e:
        sys.exit(f"analysis stopped: {e}")
