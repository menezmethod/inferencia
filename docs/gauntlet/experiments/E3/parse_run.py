"""Turn one E3 run into a results row.

Usage: python3 -I parse_run.py <stream.jsonl> <task> <arm> <accepted> <secs> <exit> <run_dir> [dispatches.jsonl]

- Claude cost: total_cost_usd from the stream's final "result" event (the
  official client's API-equivalent figure, all models included). Without one
  (timeout, crash), the cost is summed from the assistant messages' usage at
  catalog prices and cost_partial is 1. A missing cost is never 0.
- Worker usage (arm C): cc-delegate's dispatch log (DELEGATE_LOG_PATH), one line
  per delegated task, with tokens_in (fresh), cache_read_tokens,
  cache_creation_tokens, tokens_out and model_used. Falls back to tool results.
- models: models Claude used (result.modelUsage), to check arm B really ran
  subagents on its configured model.
- quota_hit: a rate_limit_event that was not "allowed": the run hit a plan limit
  and must be rerun, not counted as a quality failure.
- outside_paths: absolute paths in location fields of tool inputs (file_path, path,
  command, workdir) that point outside the run, its Go cache, ~/go and its own saved
  tool outputs (leak check). Rows with outside_paths > 0 need review.
"""
import json
import os
import re
import sys

# USD per million tokens (internal/catalog/catalog.yaml, Anthropic pricing page, 2026-10-17).
PRICE = {
    "claude-opus-5-5": (4, 20, 0.20, 5, 8),
    "claude-sonnet-5-5": (2, 10, 0.10, 2.5, 4),
    "claude-haiku-5-5": (0.10, 0.50, 0.01, 0.125, 0.20),
}
FIELDS = ["task", "arm", "accepted", "secs", "exit", "turns", "cost_usd", "cost_partial", "input", "cache_read",
          "cache_write", "output", "worker_input", "worker_cache_read", "worker_output", "worker_models",
          "models", "quota_hit", "outside_paths", "result"]
PATH_RE = re.compile(r"(?<![\w.])(/(?:Users|private|tmp|var|opt|Volumes)/[^\s\"'`;|&)<>\\]*)")
# Only fields that name a location are checked; file contents may mention any path.
PATH_FIELDS = ("file_path", "path", "notebook_path", "command", "workdir", "cwd")


def price_usage(model, u):
    p = PRICE.get(model)
    if p is None:
        return None
    one_hour = (u.get("cache_creation") or {}).get("ephemeral_1h_input_tokens", 0)
    write = u.get("cache_creation_input_tokens", 0)
    return (u.get("input_tokens", 0) * p[0] + u.get("output_tokens", 0) * p[1] + u.get("cache_read_input_tokens", 0) * p[2]
            + (write - one_hour) * p[3] + one_hour * p[4]) / 1e6


def under(path, roots):
    return any(path == r or path.startswith(r + os.sep) for r in roots)


def outside(path, allowed, sensitive):
    """True when path could reach an answer: under home or the E3 workdir, and not allowed."""
    path = os.path.normpath(path)
    return under(path, sensitive) and not under(path, allowed)


def main():
    stream, task, arm, accepted, secs, code, run = sys.argv[1:8]
    dispatch_log = sys.argv[8] if len(sys.argv) > 8 else ""
    home = os.path.expanduser("~")
    real = os.path.realpath(run)
    # Allowed: the run, the module cache, the run's own Go cache, and the run's own
    # Claude Code project dir, where the client saves large tool outputs that the
    # model then reads back (its own session's data, not another run's).
    allowed = [real, os.path.normpath(run), os.path.join(home, "go"), "/tmp", "/dev",
               os.path.join(os.path.dirname(os.path.dirname(real)), "gocache-" + os.path.basename(real)),
               os.path.join(home, ".claude", "projects", re.sub(r"[^A-Za-z0-9]", "-", real))]
    sensitive = [home, os.path.dirname(os.path.dirname(real))]  # answers live only here: source repos, clones, other runs
    result, by_msg, quota, leaks = {}, {}, 0, 0
    worker = {"input": 0, "cache_read": 0, "output": 0}
    worker_models = set()
    stream_worker = {"input": 0, "cache_read": 0, "output": 0}

    def add_worker(acc, rec):
        acc["input"] += int(rec.get("tokens_in") or 0) + int(rec.get("cache_creation_tokens") or 0)
        acc["cache_read"] += int(rec.get("cache_read_tokens") or 0)
        acc["output"] += int(rec.get("tokens_out") or 0)

    def walk(obj):
        if isinstance(obj, dict):
            if "tokens_in" in obj and "tokens_out" in obj:
                add_worker(stream_worker, obj)
                return
            for v in obj.values():
                walk(v)
        elif isinstance(obj, list):
            for v in obj:
                walk(v)

    try:
        lines = open(stream, errors="replace")
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
                    fields = [inp.get(k) for k in PATH_FIELDS if isinstance(inp.get(k), str)]
                    for bundle in inp.get("tasks") or []:  # cc-delegate batches name a workdir per task
                        if isinstance(bundle, dict) and isinstance(bundle.get("workdir"), str):
                            fields.append(bundle["workdir"])
                    for text in fields:
                        for p in PATH_RE.findall(text):
                            if outside(p.rstrip(".,:;"), allowed, sensitive):
                                leaks += 1
        elif kind == "user":
            for block in (ev.get("message") or {}).get("content") or []:
                if not (isinstance(block, dict) and block.get("type") == "tool_result"):
                    continue
                content = block.get("content")
                for part in content if isinstance(content, list) else [{"text": content}]:
                    text = part.get("text") if isinstance(part, dict) else None
                    if isinstance(text, str) and "tokens_in" in text:
                        try:
                            walk(json.loads(text))
                        except ValueError:
                            pass

    if dispatch_log and os.path.exists(dispatch_log):
        for line in open(dispatch_log, errors="replace"):
            try:
                rec = json.loads(line)
            except ValueError:
                continue
            add_worker(worker, rec)
            worker_models.add(str(rec.get("model_used") or rec.get("response_model") or rec.get("model") or "?"))
    else:
        worker = stream_worker

    cost, partial = result.get("total_cost_usd"), 0
    if cost is None:
        partial = 1
        priced = [price_usage(m, u) for m, u in by_msg.values()]
        cost = sum(p for p in priced if p is not None) if priced and None not in priced else ""
    if "limit" in str(result.get("result", "")).lower() and result.get("is_error"):
        quota = 1
    u = result.get("usage") or {}
    if partial:  # token totals from the messages seen
        u = {k: sum(v[1].get(k, 0) for v in by_msg.values()) for k in
             ("input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "output_tokens")}
    models = "|".join(sorted((result.get("modelUsage") or {}).keys())) or "|".join(sorted({m for m, _ in by_msg.values()}))
    row = [task, arm, accepted, secs, code, result.get("num_turns", ""), cost, partial,
           u.get("input_tokens", ""), u.get("cache_read_input_tokens", ""), u.get("cache_creation_input_tokens", ""),
           u.get("output_tokens", ""), worker["input"], worker["cache_read"], worker["output"],
           "|".join(sorted(worker_models)), models, quota, leaks, result.get("subtype", "none")]
    print(",".join(str(x) for x in row))


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "--header":
        print(",".join(FIELDS))
    else:
        main()
