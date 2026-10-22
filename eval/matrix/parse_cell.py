"""Turn one matrix cell into a matrix.csv row.

Usage: python3 -I parse_cell.py <stream.jsonl> <task> <model> <accepted> <visible_pass> <secs> <exit> <run_dir>
       python3 -I parse_cell.py --header

Cost (eval/README.md section 1): every cell is priced the same way, from the usage of each
assistant message at the catalog prices below (with the 1-hour cache-write split and Haiku's
long-context rule). cost_check is a second figure for a cross-check: the client's
total_cost_usd for claude-* cells, and result.modelUsage at catalog prices for gateway cells
(the client prices unknown models wrongly). A model with no price makes cost empty. It is
never 0. The analysis rejects empty costs.

invalid = 1 when the run did not reach the model's own end: an API or gateway error, no
result event without a timeout, or an error after one turn. Such a row is not a model
failure; run_matrix.sh removes it and stops.

Other fields: quota_hit (a plan limit stopped the run; rerun it, it is not a quality
failure), outside_paths (E3's leak check; a row with outside_paths > 0 needs review),
models (every model that served a call, to prove the cell stayed on one model).
"""
import json
import os
import re
import sys

# USD per million tokens: (input, output, cache_read, cache_write_5m, cache_write_1h).
# Source: internal/catalog/catalog.yaml, as_of 2026-10-17. The catalog has no cache-write
# price for the gateway models; a cache write is priced as fresh input there (not 0).
PRICE = {
    "claude-opus-5-5": (4, 20, 0.20, 5, 8),
    "claude-sonnet-5-5": (2, 10, 0.10, 2.5, 4),
    "claude-haiku-5-5": (0.10, 0.50, 0.01, 0.125, 0.20),
    "deepseek-v4-1-flash": (0.15, 0.60, 0.003, 0.15, 0.15),
    "muse-spark-1.3-contributor": (0.10, 0.20, 0.002, 0.10, 0.10),
}
# Names a gateway or client may report for the same model.
ALIASES = {
    "deepseek/deepseek-v4.1-flash": "deepseek-v4-1-flash", "deepseek-v4.1-flash": "deepseek-v4-1-flash",
    "openai-muse-spark-1-3-contrib": "muse-spark-1.3-contributor",
    "meta/muse-spark-1.3-contributor": "muse-spark-1.3-contributor",
}
# Haiku's long-context price applies per request above 100k prompt tokens.
HAIKU_LONG = (100000, (0.50, 2.50, 0.05, 0.625, 1))

FIELDS = ["task", "model", "accepted", "visible_pass", "secs", "exit", "turns", "cost_usd", "cost_check",
          "cost_source", "input", "cache_read", "cache_write", "output", "models", "quota_hit",
          "outside_paths", "result", "invalid"]
TIMEOUT_EXIT = "142"  # group_run.sh after the time limit: a real model failure
PATH_RE = re.compile(r"(?<![\w.])(/(?:Users|private|tmp|var|opt|Volumes)/[^\s\"'`;|&)<>\\]*)")
PATH_FIELDS = ("file_path", "path", "notebook_path", "command", "workdir", "cwd")


def canon(model):
    return ALIASES.get(model, model)


def price_message(model, u):
    """Price one request's usage. None when the model has no price."""
    p = PRICE.get(canon(model))
    if p is None:
        return None
    one_hour = (u.get("cache_creation") or {}).get("ephemeral_1h_input_tokens", 0) or 0
    write = u.get("cache_creation_input_tokens", 0) or 0
    fresh, read, out = u.get("input_tokens", 0) or 0, u.get("cache_read_input_tokens", 0) or 0, u.get("output_tokens", 0) or 0
    if canon(model) == "claude-haiku-5-5" and fresh + read + write > HAIKU_LONG[0]:
        p = HAIKU_LONG[1]
    return (fresh * p[0] + out * p[1] + read * p[2] + (write - one_hour) * p[3] + one_hour * p[4]) / 1e6


def price_model_usage(model_usage):
    """Price result.modelUsage (totals per model). None if any model has no price."""
    total = 0.0
    for model, u in model_usage.items():
        p = PRICE.get(canon(model))
        if p is None:
            return None
        total += ((u.get("inputTokens", 0) or 0) * p[0] + (u.get("outputTokens", 0) or 0) * p[1]
                  + (u.get("cacheReadInputTokens", 0) or 0) * p[2] + (u.get("cacheCreationInputTokens", 0) or 0) * p[3]) / 1e6
    return total


def under(path, roots):
    return any(path == r or path.startswith(r + os.sep) for r in roots)


def main(argv):
    stream, task, model, accepted, visible, secs, code, run = argv[1:9]
    home = os.path.expanduser("~")
    real = os.path.realpath(run)
    allowed = [real, os.path.normpath(run), os.path.join(home, "go"), "/tmp", "/dev",
               os.path.join(os.path.dirname(os.path.dirname(real)), "gocache-" + os.path.basename(real)),
               os.path.join(home, ".claude", "projects", re.sub(r"[^A-Za-z0-9]", "-", real))]
    sensitive = [home, os.path.dirname(os.path.dirname(real))]
    result, by_msg, quota, leaks = {}, {}, 0, 0
    try:
        with open(stream, errors="replace") as f:
            lines = f.readlines()
    except OSError:
        lines = []
    for line in lines:
        try:
            ev = json.loads(line)
        except ValueError:
            continue
        kind = ev.get("type")
        if kind == "result":
            result = ev
        elif kind == "rate_limit_event":
            if (ev.get("rate_limit_info") or {}).get("status") not in (None, "allowed", "allowed_warning"):
                quota = 1
        elif kind == "assistant":
            msg = ev.get("message") or {}
            if msg.get("id") and msg.get("usage"):
                by_msg[msg["id"]] = (msg.get("model", ""), msg["usage"])
            for block in msg.get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_use":
                    inp = block.get("input") or {}
                    for text in (inp.get(k) for k in PATH_FIELDS if isinstance(inp.get(k), str)):
                        for p in PATH_RE.findall(text):
                            p = os.path.normpath(p.rstrip(".,:;"))
                            if under(p, sensitive) and not under(p, allowed):
                                leaks += 1

    model_usage = result.get("modelUsage") or {}
    priced_msgs = [price_message(m or model, u) for m, u in by_msg.values()]
    msg_cost = sum(priced_msgs) if priced_msgs and None not in priced_msgs else None
    if model.startswith("claude-"):
        check = result.get("total_cost_usd")
    else:
        check = price_model_usage(model_usage) if model_usage else None
    if msg_cost is not None:
        cost, source = msg_cost, "messages"
    else:
        cost, source = "", "unpriced"
    text = str(result.get("result", "")).lower()
    if "limit" in text and result.get("is_error"):
        quota = 1

    subtype = result.get("subtype", "none")
    turns = result.get("num_turns", 0) or 0
    invalid = 0
    if not result and code != TIMEOUT_EXIT:
        invalid = 1  # the client died before its end (API, gateway or crash)
    elif result and subtype not in ("success", "error_max_turns"):
        invalid = 1
    elif result.get("is_error") and (turns <= 1 or "api error" in text):
        invalid = 1  # an API or gateway error (auth, model name, 5xx, outage mid-run)
    if quota:
        invalid = 1

    u = {k: sum((v[1].get(k, 0) or 0) for v in by_msg.values()) for k in
         ("input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "output_tokens")}
    # Served models come from each response's message.model, not from what the client asked for.
    served = sorted({canon(m) for m, _ in by_msg.values() if m})
    row = [task, model, accepted, visible, secs, code, result.get("num_turns", ""),
           "" if cost == "" else round(cost, 6), "" if check is None else round(check, 6), source,
           u["input_tokens"], u["cache_read_input_tokens"], u["cache_creation_input_tokens"],
           u["output_tokens"], "|".join(served), quota, leaks, subtype, invalid]
    return ",".join(str(x) for x in row)


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "--header":
        print(",".join(FIELDS))
    else:
        print(main(sys.argv))
