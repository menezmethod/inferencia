"""Tests for matrix.py. Run: python3 -I -m unittest discover -s eval/analysis"""
import csv
import io
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import matrix as M  # noqa: E402

DS, SON, OPUS, HAIKU = "deepseek-v4-1-flash", "claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-5-5"


def cell(acc, cost, vis=None):
    return {"acc": acc, "cost": cost, "vis": str(acc if vis is None else vis), "leak": 0}


def task(ds, son, opus, haiku=(0, 0.05)):
    return {DS: cell(*ds), SON: cell(*son), OPUS: cell(*opus), HAIKU: cell(*haiku)}


class Policies(unittest.TestCase):
    def test_cascade_pays_every_failed_attempt(self):
        cells = {"T1": task((0, 0.01), (1, 0.30), (1, 1.00))}
        self.assertEqual(M.cascade(cells, ["T1"], 0, "vis"), {"T1": (0.31, 1)})

    def test_cascade_visible_follows_visible_not_hidden(self):
        # Visible tests pass but the hidden grade fails: the router stops and loses the task.
        cells = {"T1": {**task((0, 0.01), (1, 0.30), (1, 1.00)), DS: cell(0, 0.01, vis=1)}}
        self.assertEqual(M.cascade(cells, ["T1"], 0, "vis"), {"T1": (0.01, 0)})
        self.assertEqual(M.cascade(cells, ["T1"], 0, "acc"), {"T1": (0.31, 1)})

    def test_cascade_from_decider_start(self):
        cells = {"T1": task((1, 0.01), (1, 0.30), (1, 1.00))}
        self.assertEqual(M.cascade(cells, ["T1"], {"T1": 2}, "vis"), {"T1": (1.00, 1)})

    def test_cascade_needs_visible_signal(self):
        cells = {"T1": {**task((0, 0.01), (1, 0.3), (1, 1.0)), DS: {"acc": 0, "cost": 0.01, "vis": "", "leak": 0}}}
        with self.assertRaises(M.DataError):
            M.cascade(cells, ["T1"], 0, "vis")

    def test_oracle_cheapest_accepted_else_opus(self):
        cells = {"T1": task((0, 0.01), (1, 0.30), (1, 1.00)), "T2": task((0, 0.01), (0, 0.30), (0, 1.00))}
        self.assertEqual(M.oracle(cells, ["T1", "T2"]), {"T1": (0.30, 1), "T2": (1.00, 0)})

    def test_decider_threshold_never_starts_fast_below_threshold(self):
        dec = {"T1": {"tier": "fast", "choice": "fast", "p": 0.2}, "T2": {"tier": "fast", "choice": "frontier", "p": 0.9}}
        self.assertEqual(M.decider_start(dec, 0.5), {"T1": 1, "T2": 0})


class Stats(unittest.TestCase):
    def test_perfect_calibration_has_zero_ece(self):
        ps = [0.1] * 10 + [0.9] * 10
        ys = [1] + [0] * 9 + [1] * 9 + [0]
        self.assertAlmostEqual(M.ece(ps, ys)[0], 0.0)

    def test_apgr_of_diagonal_is_half(self):
        self.assertAlmostEqual(M.apgr([(0.5, 0.5)]), 0.5)

    def test_pareto_drops_dominated(self):
        self.assertEqual(M.pareto({"a": (1, 0.9), "b": (2, 0.8), "c": (0.5, 0.5)}), ["c", "a"])

    def test_bootstrap_identical_policies(self):
        pol = {f"T{i}": (1.0, 1) for i in range(10)}
        s = M.paired(pol, pol, list(pol), 200, M.random.Random(1))
        self.assertEqual((s["ratio"], s["ub"], s["diff_lb"]), (1.0, 1.0, 0.0))


class Loading(unittest.TestCase):
    def write(self, text):
        f = tempfile.NamedTemporaryFile("w", suffix=".csv", delete=False)
        f.write(text)
        f.close()
        self.addCleanup(os.unlink, f.name)
        return f.name

    def test_missing_cost_is_an_error_not_zero(self):
        p = self.write("task,model,accepted,visible_pass,cost_usd,quota_hit,outside_paths\nT1,x,1,1,,0,0\n")
        with self.assertRaises(M.DataError):
            M.load_matrix(p)

    def test_quota_hit_row_is_an_error(self):
        p = self.write("task,model,accepted,visible_pass,cost_usd,quota_hit,outside_paths\nT1,x,0,0,0.1,1,0\n")
        with self.assertRaises(M.DataError):
            M.load_matrix(p)

    def test_invalid_run_is_an_error(self):
        p = self.write("task,model,accepted,visible_pass,cost_usd,quota_hit,outside_paths,invalid\nT1,x,0,0,0.1,0,0,1\n")
        with self.assertRaises(M.DataError):
            M.load_matrix(p)

    def test_missing_decider_latency_is_an_error(self):
        p = self.write("decider,task,start_tier,p_fast,tier_choice,latency_ms,cost_usd\nlaya,T1,fast,0.6,strong,,0\n")
        with self.assertRaises(M.DataError):
            M.load_decisions([p])

    def test_cost_check_mismatch_is_flagged(self):
        p = self.write("task,model,accepted,visible_pass,cost_usd,cost_check,quota_hit,outside_paths,invalid\n"
                       "T1,x,1,1,1.00,1.20,0,0,0\nT2,x,1,1,1.00,1.02,0,0,0\n")
        _, flags = M.load_matrix(p)
        self.assertEqual(flags["cost check differs by more than 5%"], ["T1/x"])

    def test_incomplete_and_leaky_tasks_are_excluded(self):
        flags = M.defaultdict(list)
        cells = {"T1": task((1, 0.01), (1, 0.3), (1, 1.0)), "T2": {OPUS: cell(1, 1.0)},
                 "T3": {**task((1, 0.01), (1, 0.3), (1, 1.0)), OPUS: {**cell(1, 1.0), "leak": 2}}}
        self.assertEqual(M.complete_tasks(cells, flags), ["T1"])
        self.assertEqual(len(flags), 2)


class EndToEnd(unittest.TestCase):
    def test_cheap_model_that_always_passes_is_the_best_fixed_policy(self):
        cells = {f"T{i}": task((1, 0.01), (1, 0.30), (1, 1.00)) for i in range(12)}
        suite = {t: "inferencia" for t in cells}
        dec = {"rules": {t: {"tier": "strong", "choice": "", "p": None, "ms": 1, "cost": 0} for t in cells}}
        report = M.analyze(cells, suite, dec, M.defaultdict(list), boot=200)
        self.assertIn(f"Best fixed policy that holds the quality floor: **always {DS}**", report)
        # Rules start at strong: 30x the cost of always-deepseek, so G1 fails.
        self.assertIn("| G1 | rules |", report)
        self.assertRegex(report, r"\| G1 \| rules \|.*\| FAIL \|")
        self.assertRegex(report, r"\| G1 \| cascade-visible \|.*\| FAIL \|")

    def test_cascade_wins_when_cheap_model_passes_half(self):
        cells = {f"T{i}": task((i % 2, 0.01), (1, 0.30), (1, 1.00), haiku=(0, 0.05)) for i in range(40)}
        suite = {t: "inferencia" for t in cells}
        report = M.analyze(cells, suite, {}, M.defaultdict(list), boot=500)
        self.assertIn("Best fixed policy that holds the quality floor: **always claude-sonnet-5-5**", report)
        self.assertRegex(report, r"\| G1 \| cascade-visible \|.*\| PASS \|")


if __name__ == "__main__":
    unittest.main()
