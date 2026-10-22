"""Tests for parse_cell.py. Run: python3 -I -m unittest discover -s eval/matrix"""
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import parse_cell as P  # noqa: E402


def stream(events):
    f = tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False)
    for e in events:
        f.write(json.dumps(e) + "\n")
    f.close()
    return f.name


def row(path, model):
    run = tempfile.mkdtemp()
    return dict(zip(P.FIELDS, P.main(["x", path, "T01", model, "1", "1", "10", "0", run]).split(",")))


class ParseCell(unittest.TestCase):
    def test_gateway_cost_from_model_usage_ignores_client_total(self):
        p = stream([{"type": "result", "total_cost_usd": 9.99, "num_turns": 3, "subtype": "success",
                     "modelUsage": {"deepseek-v4-1-flash": {"inputTokens": 1_000_000, "outputTokens": 1_000_000,
                                                            "cacheReadInputTokens": 1_000_000, "cacheCreationInputTokens": 0}}}])
        r = row(p, "deepseek-v4-1-flash")
        self.assertAlmostEqual(float(r["cost_usd"]), 0.15 + 0.60 + 0.003)
        self.assertEqual(r["cost_source"], "model_usage")

    def test_gateway_alias_is_priced(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "openai-muse-spark-1-3-contrib",
                     "usage": {"input_tokens": 1_000_000, "output_tokens": 0}}}])
        r = row(p, "muse-spark-1.3-contributor")
        self.assertAlmostEqual(float(r["cost_usd"]), 0.10)
        self.assertEqual(r["models"], "muse-spark-1.3-contributor")

    def test_unknown_model_cost_is_empty_never_zero(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "mystery", "usage": {"input_tokens": 5}}}])
        r = row(p, "deepseek-v4-1-flash")
        self.assertEqual((r["cost_usd"], r["cost_source"]), ("", "unpriced"))

    def test_claude_cell_uses_client_total_and_records_token_cost(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "claude-sonnet-5-5",
                     "usage": {"input_tokens": 1_000_000, "output_tokens": 0}}},
                    {"type": "result", "total_cost_usd": 2.0, "num_turns": 1, "subtype": "success", "usage": {}}])
        r = row(p, "claude-sonnet-5-5")
        self.assertEqual((r["cost_usd"], r["cost_source"], r["cost_tokens"]), ("2.0", "client", "2.0"))

    def test_haiku_long_context_price(self):
        u = {"input_tokens": 0, "cache_read_input_tokens": 200_000, "output_tokens": 0}
        self.assertAlmostEqual(P.price_message("claude-haiku-5-5", u), 200_000 * 0.05 / 1e6)

    def test_quota_event(self):
        p = stream([{"type": "rate_limit_event", "rate_limit_info": {"status": "rejected"}}])
        self.assertEqual(row(p, "claude-haiku-5-5")["quota_hit"], "1")


if __name__ == "__main__":
    unittest.main()
