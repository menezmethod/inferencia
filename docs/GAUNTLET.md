# Inferencia v4 gauntlet

Paste this one line as the goal in a long-running agent (a Command Code goal, a Claude Code
session, or a Codex session):

```text
Run the gauntlet in docs/GAUNTLET.md exactly as written. Resume from docs/gauntlet/STATE.md. Re-read docs/GAUNTLET.md every time you start or resume.
```

The run survives pauses, for example the 100-turn limit on a Command Code goal. All state lives
in `docs/gauntlet/STATE.md`, so to resume you paste the same line again.

Before you start:

1. `BUDGET_USD` is set to $25. It is the maximum paid API spend; plan quota does not count. The
   design keeps the expensive part on quota: every Gate 1 arm runs in an official client under
   your own plans, so paid API spend goes only to cheap delegate workers and the small gateway
   tests (E5-E8). For scale (a guess, not measured): paying per token for a strong model costs
   about $0.50-$3 per coding task, so the 3-arm pilot would cost $45-$270 on API keys. The
   powered set (124-185 tasks x 3 arms) takes days of plan quota, so expect Gate 1 to land as
   PROVISIONAL by the 2026-11-11 deadline.
2. Export the API keys the experiments may use (for example `OPENROUTER_API_KEY`,
   `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`) in the shell that runs the agent.
   Never paste keys into the goal.
3. Check that `docs/PLAN_V4.md` exists. It is the plan the gauntlet builds against.

---

## Instructions

```text
GOAL: Build inferencia v4 as docs/PLAN_V4.md specifies, prove or disprove its cost claims with
real experiments, and finish with a senior-developer education document. Work as a gauntlet:
every stage must survive an adversarial review and a hard exit check before the next stage starts.

BUDGET_USD = 25
CC_DELEGATE = ~/AI/cc-delegate   (the existing delegation tool; its MCP entry is bin/run-mcp.sh)

== 0. RESUME PROTOCOL (do this first, every time you start or resume) ==
1. Read docs/gauntlet/STATE.md. If it does not exist, do stage S0.
2. Re-read docs/GAUNTLET.md (this file), docs/PLAN_V4.md and CLAUDE.md.
3. Run: git status; git branch --show-current.
4. Continue from the first item in STATE.md that is not DONE.

== 1. HARD RULES (a violation stops the run) ==
1. Work only on branch gauntlet/v4. Never push to main. A push to main deploys to production.
2. Never send a subscription credential or OAuth token through inferencia. Send work to a
   subscription only through that plan's own official client (claude -p, codex exec,
   opencode run), with no inferencia in the request path. Any run whose requests pass through
   inferencia uses API keys and counts toward BUDGET_USD.
3. Keep total paid API spend at or below BUDGET_USD. Record every paid call in
   docs/gauntlet/spend.csv (time, provider, model, tokens, usd, experiment). Before each
   experiment, write its spend estimate in STATE.md. If the estimate puts the total above
   BUDGET_USD, do not run it. Write SKIPPED-BUDGET.
4. Never weaken, skip or delete a test to make it pass. Never mark an item DONE without an
   evidence file. Never report a number that you did not measure. Label estimates "estimate".
5. Never write a secret to a log, a fixture, a commit or a document. Run make check-sensitive
   before each commit. Redact keys to the last 4 characters.
6. Go style: stdlib net/http, no frameworks. Small interfaces (1-2 methods). Accept interfaces,
   return structs. Errors are values: wrap them with %w, do not panic. Pass context.Context
   first. Clear is better than clever. Add a dependency only with a one-paragraph
   justification in docs/gauntlet/DECISIONS.md. A pure-Go SQLite driver is an acceptable
   dependency.
7. DRY: one canonical request type, one usage record type, one price function. If you write the
   same logic twice, stop and extract it.
   Core shape (from docs/PLAN_V4.md, "Code shape"): one interface,
   Provider { Do(ctx, *Request) (*Response, error) }, plus the Request, Response, Usage and
   Candidate structs and a Route func type. Replace the six-method Backend interface in
   internal/backend/backend.go with Provider. Do not create: ProviderFactory, PluginRegistry, a
   Middleware interface, a Cache interface, a Repository over SQLite, an EventBus, a workflow
   engine abstraction, a Translator class tree. A Scorer is a func type until a second real
   scorer exists.
8. If an item fails 3 times in a row, write BLOCKED with the cause in STATE.md and go to the next
   item that does not depend on it.
9. Privacy. The harness logs hold other projects. Evidence files and CSVs hold aggregates only:
   counts, tokens, dollars, model names. Never copy transcript text into the repo. Test fixtures
   are synthetic or scrubbed. Never open auth.json files, credential tables or account tables
   (in opencode.db: credential, account, control_account, account_state).
10. Sandbox. Run E3 tasks, hidden tests and any public-repo code in a throwaway checkout or a
   container, with no home-directory credentials and no access to ~/.ssh, ~/.claude.json or
   auth files.

== 2. THE LOOP (one iteration per checklist item) ==
1. Pick the next item from STATE.md. Set it to IN-PROGRESS.
2. Write the test first. The test must fail for the right reason.
3. Implement the minimum code that makes the test pass.
4. Run: gofmt -l . (must print nothing); make vet; make test (it runs with -race).
5. ADVERSARIAL REVIEW. Start a fresh subagent with no context. If you cannot start one, do the
   review yourself as a hostile reviewer. Give the reviewer the diff and this checklist:
   - Does the test prove the claim, or only exercise the code?
   - What input breaks it? Try: empty, huge, unicode, a stream cut mid-chunk, a 429, a timeout,
     a slow upstream, a malformed upstream response, concurrent requests.
   - Is any secret, prompt text or PII logged?
   - Is there an interface with one implementation, or a config value that never changes?
   - Is logic duplicated elsewhere in the repo?
   - Does it change behavior for existing /v1/chat/completions clients?
6. Fix every finding that is real. Write rejected findings and the reason in the evidence file.
7. Write docs/gauntlet/evidence/<item-id>.md: what was built, the commands you ran, the output,
   the review findings, and the decisions.
8. Commit with a conventional message (feat:, fix:, test:, docs:). Set the item to DONE.

== 3. STAGES AND EXIT CHECKS ==

S0 SETUP
- Create docs/gauntlet/STATE.md with every item below as a checklist (id, status, evidence link).
- Switch to branch gauntlet/v4 (it already exists and holds this file). Record the baseline:
  make test and make vet output.
- Settled facts (do not re-check): Codex is logged in with a ChatGPT subscription, so Codex work
  goes only through codex exec and is never routed. Command Code's custom provider speaks
  Anthropic Messages. Still open, check in S0: whether OpenCode Go bills flat or per token (read
  OpenCode's own docs; write the answer and its URL in DECISIONS.md).
- Exit: the baseline is green, or the failures are written down as pre-existing.

S1 MEASURE (plan phase P-1)
- One Usage record struct. Adapters that read: Claude Code (~/.claude/projects/**/*.jsonl,
  deduplicate by message id), Codex CLI (~/.codex/sessions/**/*.jsonl, token_count events),
  OpenCode (~/.local/share/opencode/opencode.db, message table, role=assistant), Command Code
  (~/.commandcode/projects/**/*.jsonl, usage field). Each adapter is a function that returns
  []Usage. Read-only: never write to these files.
- Where a harness records dollars (OpenCode, Command Code), store them as "harness-computed".
  They are billed dollars only if that upstream bills per token. Use catalog prices elsewhere,
  and label them "estimate".
- Command: inferencia measure [--since --until] prints spend by harness, upstream and model,
  cache-read share, and solo vs delegated.
- Exit, golden numbers. Each source has a frozen cutoff and a fixed method. The agent's own runs
  after the cutoff must not change the result. Measure must reproduce each number within 2%:
  * OpenCode, --until 2026-09-21T13:41:18Z: rows in the message table where
    json_extract(data,'$.role')='assistant'. Expect 8,733 calls; sum of $.cost = $51.95;
    cache-read share = sum($.tokens.cache.read) / (sum($.tokens.input) + sum($.tokens.cache.read))
    = 98.2%.
  * Codex, --until 2026-10-16T04:22:03Z: the last total_token_usage event in each session file
    (8 files have one). Expect cache-read share = sum(cached_input_tokens) / sum(input_tokens)
    = 96.1%; input tokens = 444.9M.
  * Command Code, --until 2026-10-06T00:10:34Z: lines with a "usage" field in
    projects/**/*.jsonl, excluding *checkpoints*. Expect 504 calls and $3.33 (sum of costUsd).
    inputTokens includes cache reads (0 of 504 records have cacheReadTokens > inputTokens).
  * Claude Code, --until 2026-09-30T23:59:59Z: assistant messages with usage, deduplicated by
    message id. Expect about 12,247 calls. This number came from a different script, so a gap
    above 2% is a finding to explain, not a test failure.
  If a number does not match, find the cause and write it down. Do not change the expected
  number to match.

S2 CATALOG AND SAVINGS (plan phase P0)
- catalog.yaml: for each model, tier, input, output and cache-read price, data policy, source
  URL and as-of date. Fetch every price from the provider's pricing page. A price without a
  source URL is invalid.
- One price function: effective cost = (uncached input x input price + cached input x
  input price x cache-read multiplier + cache writes x input price x cache-write multiplier +
  output x output price) x (1 + retry rate). A model with no catalog price is an error. Never
  count it as $0.
- inferencia savings: actual cost vs counterfactual cost under other models, with a range for
  tokenizer differences.
- inferencia subs: for each plan, the API-equivalent value of the tokens used under it, and the
  share of quota used where the quota is known.

S3 EXPERIMENTS (the proof; spend counts toward BUDGET_USD)
Each experiment writes docs/gauntlet/experiments/<id>/ with: README.md (hypothesis, method,
result, threats to validity), data.csv, and the exact command to reproduce it.
Gate 1 must use only things that exist before S4: today's binary, measure, the catalog, the
harnesses and CC_DELEGATE. E5-E9 test the gateway, so they belong to the S4 exit check.
- E1 Ledger accuracy against a bill, not against a harness. A harness computes its dollars from
  its own price table, so ledger vs harness proves only that both use the same prices. Compare
  the ledger with a billed source: OpenRouter's per-generation cost or activity export for the
  OpenRouter rows, and provider usage APIs where a key exists. Pass: within 3%. If no billed
  source is reachable, write SKIPPED-NO-BILL and say so in S6.
- E2 Offline replay (no spend). Re-price every recorded session under these policies: as-run;
  cheapest model at the quality floor; subscription-first; main loop on a strong-tier model.
  When a policy switches provider inside a session, charge the cache loss: the next turn pays
  full input price plus a cache write. Report dollars per policy as estimates.
- E3 Live A/B on a task suite, in two steps.
  Tasks: revert commits that added a feature plus its tests, in this repo and in small public Go
  repos. The task is to rebuild the feature. The hidden tests are the grader. Record the repo of
  each task, because errors are clustered by repo.
  Arms (all exist today, no gateway needed):
  A = solo: one strong model in its official client under your plan (claude -p on the strongest
      Claude model), no inferencia (the baseline).
  B = routed by harness config: the same harness, with its own small-model and subagent-model
      settings set to what the plan's routing rules would pick. E2 replay covers what config
      cannot express.
  C = delegated: CC_DELEGATE plans once into 3-6 bundles, cheap workers build, the verify
      commands decide.
  Every task runs in every arm (paired design). Arms that run in an official client under a plan
  spend quota, not dollars; record their API-equivalent cost from the tokens.
  Metrics per arm: cost per accepted task (accepted = hidden tests pass), accept rate,
  wall-clock time, tokens, cache-read share.
  E3a PILOT: 30 tasks. Report a paired bootstrap 95% confidence interval on the cost ratio, and
  the discordance rate (the share of tasks where the arms disagree on accept).
  E3b POWERED: compute the paired sample size for a 10-point non-inferiority margin from the
  pilot discordance (one-sided alpha 0.05, power 0.8). Expect about 124-185 tasks. To keep spend
  inside BUDGET_USD, run arms in official clients under your own plans where possible (hard
  rule 2: no inferencia in that request path). If the budget or quota
  cannot cover the powered set, do not run a smaller set and call it proof. Write
  "underpowered: n=<n>, rules out only a drop larger than <x> points".
- E4 Subscription value. From the logs: API-equivalent dollars per $ paid for each plan, per
  month. Simulation: with the known quotas, how much per-token spend subscription-first dispatch
  would have avoided. Optional live part: send 5 bundles through each plan's official client
  and record the quota used.

S4 EXIT EXPERIMENTS (run against the S4 build; listed here so all experiments share one format)
- E5 Gateway overhead. Added time to first token and total latency, p50 and p99, against a local
  mock upstream (1,000 requests) and one real provider (20 requests). Target: under 5 ms p50
  added on the local mock.
- E6 Fault injection. A mock upstream returns 429, 500, a timeout, a stream cut mid-tool-call and
  malformed JSON. Pass: failover to an equal-tier model, the cache loss is logged, and the
  client sees a valid response or a valid OpenAI or Anthropic error envelope. Never a hang.
- E7 Cache preservation. A 20-turn session through inferencia vs direct, same model. Pass: the
  cache-read share through inferencia is within 2 points of direct.
- E8 Conformance. Recorded fixtures for each ingress-egress pair. Streaming events, tool calls,
  partial tool JSON, usage blocks and thinking or thought signatures survive the round trip.
- E9 Security. Tests prove: keys never appear in logs; a sensitive project never reaches a model
  marked trains or unknown; text in tool results cannot change routing, headers or the
  upstream URL; an upstream base URL cannot point at a private address unless the config allows
  it; govulncheck is clean; an SBOM is generated.

GATE 1 (after E1-E4)
- Write docs/gauntlet/GATE1.md: does a cross-provider route or delegation beat the solo baseline
  on cost per accepted task at an accept rate that is not inferior? Give the numbers, the
  confidence intervals and the sample size.
- PASS: the cost-ratio upper bound is below 1.0 AND E3b shows non-inferiority. Do S4 and S5.
- PROVISIONAL: E3b is underpowered, but the pilot shows a clear cost win: the cost-ratio 95%
  upper bound is below 0.9, and the winning arm's accept rate is no more than 5 points below the
  baseline. Do S4 in SHADOW MODE ONLY: the gateway logs the route it would pick but forwards to
  the client's chosen model. Do not do S5. Mark every claim in S6 "provisional, underpowered".
- FAIL: anything else.
- Deadline: write GATE1.md by 2026-11-11 (the plan's kill date), with whatever E3 has reached.
  E3b may continue after that date only if the pilot's cost-ratio upper bound is below 0.9.
- FAIL: skip S4 and S5, and write why. Then do S6. A fail is a valid result.

S4 GATEWAY (plan phase P1, only after Gate 1 PASS)
- Anthropic Messages ingress next to the existing OpenAI Chat ingress. Two egress adapters:
  Anthropic native and OpenAI-compatible.
- Byte-stable pass-through: when the ingress format equals the egress format, forward the raw
  body unchanged (httputil.ReverseProxy) and parse only the usage. Translate only across formats.
- Sticky routing: session key = the client's session_id, else a hash of the first system and
  first user message; 10-minute idle expiry.
- Failover with a circuit breaker on live traffic (cool a deployment after 3 failures), beside
  the existing 30 s watchdog. Typed errors: ErrRateLimited, ErrQuota, ErrContextTooLong.
- Fail-open means a fallback to the default provider. It never skips auth or budgets.
- Budgets, shadow mode, inferencia why <id>, inferencia off/on.
- OpenTelemetry GenAI attributes (gen_ai.operation.name, gen_ai.provider.name, request and
  response model, usage tokens with cache reads) and the route reason, on the existing spans
  and on the canonical log line. No prompt text in logs or spans by default.
- E5-E9 must pass against this build.

S5 DELEGATE ENGINE (plan phase P2, only after Gate 1 PASS)
- Port cc-delegate to Go: plan once into 3-6 bundles; inferencia runs the verify commands;
  the strong model reviews the diff only on risk or missing tests.
- Durable steps without Temporal: a SQLite event log. Each step (plan, worker, verify, review)
  is an idempotent row keyed by (job, step, attempt). Store a step's result before the next
  step starts, so a crash or a resume never spends tokens twice. Test it: kill the process
  mid-job, restart it, and prove that no completed step runs again.
- Verify commands come only from a repo allowlist file, never from model output. Run them in a
  sandbox with no secrets in the environment and no network.
- Subscription-first dispatch through official clients. The scorer is a func type; the rules
  scorer is the default.
- GATE 2: delegated beats solo on bench replay. Write docs/gauntlet/GATE2.md.

S6 EDUCATION DOCUMENT (always the last stage, also after a gate FAIL or a budget stop)
Write docs/EDUCATION.md for a senior developer who did not build inferencia. Contents:
1. What inferencia is and is not, in one paragraph.
2. Architecture: a Mermaid diagram from harness to ingress, canonical core, router, egress and
   upstreams, and a second diagram of one request's lifecycle, streaming included.
3. Design choices: for each, the choice, the alternatives rejected, the pros, the cons, and the
   evidence. Include: canonical format (N + M adapters); per-session sticky routing; rules
   before a learned scorer; SQLite ledger; Temporal vs a SQLite log; subscription-first
   dispatch through official clients; fail-open.
4. The Go interfaces: each interface, why it is that small, and which interfaces do not exist on
   purpose.
5. The experiments: a table and a chart for each, with the real numbers, links to the evidence
   folders, and threats to validity. State plainly what was disproved.
6. Inferencia vs no inferencia: cost per accepted task, accept rate and latency, side by side.
7. Subscriptions: the API-equivalent value each $20 plan delivered, and where spill-over to
   per-token APIs starts.
8. Security model: threats (OWASP LLM Top 10) and the control and test for each one.
9. Things a senior developer should ask that nobody asked: failure modes, operating cost,
   vendor-change risk, what breaks at 10x traffic, how to delete the project cleanly.
10. Open risks and the next three experiments to run.
Every number in the document links to an evidence file or a CSV. No number without a source.

== 4. STOP CONDITIONS ==
Stop when one of these is true, and make sure S6 is done before you stop:
- Every item is DONE, BLOCKED or SKIPPED, and S6 is done.
- The budget is used up.
- A hard rule was violated. Write the violation in STATE.md first.

== 5. FINAL REPORT ==
In the last message, write: stages done, gate results with numbers, spend vs budget, the
location of docs/EDUCATION.md, and the three most important findings.
```
