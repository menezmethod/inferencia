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


def row(path, model, code="0"):
    run = tempfile.mkdtemp()
    return dict(zip(P.FIELDS, P.main(["x", path, "T01", model, "1", "1", "10", code, run]).split(",")))


RESULT_OK = {"type": "result", "num_turns": 5, "subtype": "success", "is_error": False}


class ParseCell(unittest.TestCase):
    def test_cost_from_messages_and_gateway_check_from_model_usage(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "deepseek-v4-1-flash",
                     "usage": {"input_tokens": 1_000_000, "output_tokens": 1_000_000, "cache_read_input_tokens": 1_000_000}}},
                    {**RESULT_OK, "total_cost_usd": 9.99,
                     "modelUsage": {"deepseek-v4-1-flash": {"inputTokens": 1_000_000, "outputTokens": 0}}}])
        r = row(p, "deepseek-v4-1-flash")
        self.assertAlmostEqual(float(r["cost_usd"]), 0.15 + 0.60 + 0.003)
        self.assertAlmostEqual(float(r["cost_check"]), 0.15)
        self.assertEqual((r["cost_source"], r["invalid"]), ("messages", "0"))

    def test_served_model_comes_from_responses(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "openai-muse-spark-1-3-contrib",
                     "usage": {"input_tokens": 1_000_000, "output_tokens": 0}}},
                    {"type": "assistant", "message": {"id": "m2", "model": "deepseek-v4-1-flash",
                     "usage": {"input_tokens": 0, "output_tokens": 0}}},
                    {**RESULT_OK, "modelUsage": {"openai-muse-spark-1-3-contrib": {}}}])
        r = row(p, "muse-spark-1.3-contributor")
        self.assertEqual(r["models"], "deepseek-v4-1-flash|muse-spark-1.3-contributor")

    def test_unknown_model_cost_is_empty_never_zero(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "mystery", "usage": {"input_tokens": 5}}}, RESULT_OK])
        r = row(p, "deepseek-v4-1-flash")
        self.assertEqual((r["cost_usd"], r["cost_source"]), ("", "unpriced"))

    def test_claude_cell_check_is_client_total(self):
        p = stream([{"type": "assistant", "message": {"id": "m1", "model": "claude-sonnet-5-5",
                     "usage": {"input_tokens": 1_000_000, "output_tokens": 0}}},
                    {**RESULT_OK, "total_cost_usd": 2.0}])
        r = row(p, "claude-sonnet-5-5")
        self.assertEqual((r["cost_usd"], r["cost_check"], r["cost_source"]), ("2.0", "2.0", "messages"))

    def test_one_hour_cache_write_rate(self):
        u = {"input_tokens": 0, "cache_creation_input_tokens": 1_000_000,
             "cache_creation": {"ephemeral_1h_input_tokens": 1_000_000}}
        self.assertAlmostEqual(P.price_message("claude-opus-5-5", u), 8.0)

    def test_haiku_long_context_price(self):
        u = {"input_tokens": 0, "cache_read_input_tokens": 200_000, "output_tokens": 0}
        self.assertAlmostEqual(P.price_message("claude-haiku-5-5", u), 200_000 * 0.05 / 1e6)

    def test_quota_event_is_invalid(self):
        p = stream([{"type": "rate_limit_event", "rate_limit_info": {"status": "rejected"}}, RESULT_OK])
        r = row(p, "claude-haiku-5-5")
        self.assertEqual((r["quota_hit"], r["invalid"]), ("1", "1"))

    def test_api_error_is_invalid_not_a_model_failure(self):
        p = stream([{"type": "result", "num_turns": 1, "subtype": "success", "is_error": True,
                     "result": "API Error: 502 Bad Gateway"}])
        self.assertEqual(row(p, "deepseek-v4-1-flash")["invalid"], "1")

    def test_no_result_event_is_invalid_unless_timeout(self):
        p = stream([])
        self.assertEqual(row(p, "deepseek-v4-1-flash")["invalid"], "1")
        self.assertEqual(row(p, "deepseek-v4-1-flash", code="142")["invalid"], "0")

    def test_max_turns_is_a_model_failure_not_invalid(self):
        p = stream([{"type": "result", "num_turns": 80, "subtype": "error_max_turns", "is_error": True}])
        self.assertEqual(row(p, "deepseek-v4-1-flash")["invalid"], "0")


if __name__ == "__main__":
    unittest.main()
