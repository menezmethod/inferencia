<!-- Snapshot of the "Inferencia v4" plan doc, exported 2026-10-17. The live doc is the source of truth: https://claude.ai/code/artifact/a64e4f3e-d3c3-44c1-8395-074617297e52 -->

# Inferencia v4: measure first, then route any model

Inferencia is a cost router that sits under any harness, not a harness. Its one job: the lowest cost per accepted task from the best model available, on any provider or gateway: Anthropic, OpenAI, Google, DeepSeek, OpenRouter, OpenCode Zen and Go, local Ollama or any OpenAI-compatible host. Build a read-only, cross-provider cost ledger before any gateway. Your logs from four harnesses say the same thing: 92-99% of input is cache reads on the models you use most, and, where checked, near zero small-model chatter. So the money is in which model runs the main loop and how big its context gets, not in rerouting small calls.

## The decision

1. **Week 1: `inferencia measure`.** Reads Claude Code, Codex CLI, OpenCode and Command Code logs, plus provider usage exports where a CLI logs no tokens. Reports spend by provider and model, cache share, and solo vs delegated. No proxy, works with any gateway.
2. **Weeks 2-3: test routing and delegation on your own tasks, across providers.** Planner and worker can be any model in any harness. Keep cc-delegate as a script; the ledger scores cost per accepted task.
3. **Week 3 gate:** build the gateway and delegate engine only if a cross-provider route or delegation measurably wins. Otherwise ship the ledger alone.
4. **Kill date: 2026-11-11** (6 weeks from 2026-09-30). No date extension without gate evidence. Decided 2026-10-17: the date holds for the Gate 1 decision (measure, replay and a 30-task pilot). Build dates move only on gate evidence.

North star, unchanged in spirit: **cost per accepted task at the quality of the best single model**, on any provider, orchestration overhead included.

## Where inferencia sits

Inferencia is a base-URL swap under the harness the user already runs. It never owns the agent loop, tools or UI.

| Harness or client | Wire format it speaks | Plugs in by | Ingress ships |
| --- | --- | --- | --- |
| OpenCode | Per provider (OpenAI-compatible or Anthropic) | Custom provider with a base URL | Exists today (OpenAI-compatible) |
| Cursor, Aider, apps on OpenAI SDKs | OpenAI Chat Completions | Base-URL override | Exists today (`/v1/chat/completions`) |
| Claude Code, apps on Anthropic SDKs | Anthropic Messages | `ANTHROPIC_BASE_URL` | P1 |
| Command Code | Anthropic Messages (custom provider api: anthropic-messages) | Custom provider entry (yours points at codex-router) | P1 |
| Codex CLI | OpenAI Responses | `model_providers` base URL (your codex-router already does this) | P2 |
| Gemini CLI | Gemini native | Base-URL override, not verified | P3 |

Inside, one canonical request format. Each ingress and each egress is one adapter, so N harnesses and M providers cost N + M adapters, not N x M. Egress starts with two adapters: Anthropic native, and OpenAI-compatible. The second covers OpenAI, Gemini's OpenAI-compatible endpoint, DeepSeek, OpenRouter and Ollama.

Upstream, inferencia treats gateways as providers. It does not compete with them; it picks between them and measures them.

| Upstream | Billing | Egress adapter | How inferencia treats it |
| --- | --- | --- | --- |
| Anthropic, OpenAI, Google, DeepSeek APIs | Per token | Anthropic native; OpenAI-compatible for the rest | Direct providers in the catalog |
| OpenRouter | Per token, many models | OpenAI-compatible | One provider with many models. Its own provider routing stays on; the ledger records which upstream served each call. |
| OpenCode Zen | Per token, curated models | OpenAI-compatible | Same as OpenRouter |
| OpenCode Go | Subscription, not verified | OpenAI-compatible | If it is a subscription: tasks go to OpenCode itself under the plan; credentials never pass through inferencia |
| Vercel AI Gateway, LiteLLM proxy, Together, Fireworks, Groq | Per token | OpenAI-compatible | Catalog entries; LiteLLM and Vercel logs are also ledger inputs |
| Bedrock, Vertex, Azure OpenAI | Per token, cloud account | Native adapters | P3, only if a user needs cloud billing |
| Ollama (local or cloud) | Free locally | OpenAI-compatible (exists) | The `local` tier |

The same model often sits behind two upstreams: you run glm-5.2 through both OpenCode Go and OpenRouter. Comparing those two on cost and accept rate is exactly the ledger's job.

## What your own data says

Claude Code (Anthropic), read 2026-09-30: 376 transcript files in `~/.claude/projects` (12,247 unique calls). Shares use assumed list-price weights, so they are relative, not billed dollars.

| Model | Calls | Share of est. spend | Cache-read tokens |
| --- | --- | --- | --- |
| claude-sonnet-5 | 9,462 | 48.2% | 2.76B |
| claude-opus-5-5 | 1,489 | 40.4% | 482M |
| claude-opus-5 | 682 | 9.3% | 81M |
| claude-sonnet-5-5 | 614 | 2.1% | 116M |
| Haiku-class | 0 | 0% | none logged |

Codex CLI (OpenAI), read 2026-10-17: 48 session files in `~/.codex/sessions`, 8 with token counts. No price weights applied, so this table shows tokens only.

| Model | Sessions | Input tokens | Cache-read share | Output tokens |
| --- | --- | --- | --- | --- |
| gpt-6-astra | 3 | 384M | 96.3% | 1.60M |
| gpt-6.1-sol | 2 | 57.8M | 94.8% | 105k |
| gpt-5.6-luna | 1 | 1.77M | 86.7% | 9k |
| gpt-5.4 | 2 | 1.46M | 92.4% | 18k |

Gemini CLI: 16 chat files in `~/.gemini/tmp`, but they hold no token counts. Measuring it needs telemetry, a usage export or the gateway path.

OpenCode, read 2026-10-17: `opencode.db`, 8,733 assistant calls, $51.95 logged cost. These are dollars the harness computes per call, not guessed weights. They are billed dollars only where the upstream bills per token.

| Upstream / model | Calls | Cache-read share | Logged cost |
| --- | --- | --- | --- |
| OpenCode Go / glm-5.2 | 629 | 91.8% | $19.96 |
| OpenCode Go / kimi-k3 | 207 | 82.8% | $15.05 |
| OpenRouter / z-ai/glm-5.2 | 333 | 92.1% | $7.41 |
| OpenCode Go / deepseek-v4-flash | 6,542 | 99.4% | $5.90 |
| OpenCode Go / deepseek-v4.1-flash | 915 | 92.2% | $2.84 |
| OpenCode Go / mimo-v2.5-pro | 59 | 80.3% | $0.78 |

Command Code, read 2026-10-17: 10 session logs, 504 calls, $3.33 logged cost.

| Model | Calls | Cache-read share | Logged cost |
| --- | --- | --- | --- |
| meta/muse-spark-1.3-contributor | 193 | 30.3% | $2.73 |
| deepseek/deepseek-v4.1-flash | 311 | 96.9% | $0.60 |

Two findings from the logged dollars. In OpenCode, glm-5.2 and kimi-k3 take 82% of cost on 13% of calls; deepseek-v4-flash runs 75% of calls for 11% of cost. In Command Code, muse-spark costs 4.5x deepseek on fewer calls. Its cache hit rate is 30% against 97%, and its per-token prices differ too. Cache hit rate per upstream belongs in the catalog and the routing score.

Cache reads are 98.2% of input in Claude Code, 96.1% in Codex and 98.2% in OpenCode. Three consequences:

- **Chatter rerouting addresses about 0% of your visible spend in Claude Code and Codex.** Harnesses carry their own small-model setting (Claude Code has `ANTHROPIC_DEFAULT_HAIKU_MODEL`), so no gateway is needed for that lever.
- **The real levers are the main-loop model and context size, on every provider.** Cache reads scale with context length times turns. Delegating build work keeps the expensive model's context small, which is the one saving the data supports.
- **Reasoning effort is a third, smaller lever.** About 70% of Codex output tokens are reasoning tokens at effort `high`. Output is small next to input, but the setting costs nothing to change.

Caveats: one user, four harnesses, guessed price weights, and background calls may not appear in logs. Your Codex traffic already goes through your local codex-router; codex login status says it is logged in with ChatGPT, so it is a subscription. Inferencia measures it and never re-routes it; Codex work goes only through codex exec. This is a hypothesis generator, not proof.

## What two adversary rounds changed, and what going multi-provider adds

| Objection | Change in v4 |
| --- | --- |
| Plan/build/verify cost 3x solo in a public benchmark (14 dispatches, about $11 of planner tokens) | Few big bundles; inferencia runs the verify commands itself; the strongest model reviews the diff only on risk or missing tests. Gated on your own data. |
| Subscription logins through third-party tools break consumer terms (Anthropic, Feb 2026; OpenAI and Google not verified) | Target user pays per token on any provider. Claude, ChatGPT, Google-account and OpenCode Go plans are never proxied. Inferencia measures them and can send whole tasks to each plan's own official client (see Subscriptions). |
| Switching model mid-session destroys the prompt cache; across providers the cache is always lost | Route per session, subagent or background call. Never per turn. Cross-provider switch only on 429, quota or outage, and the ledger logs the cache loss. |
| Protocol translation is the real project, and harder with many providers | One canonical format, N + M adapters. P1 routes within a wire format: Anthropic in to Anthropic models, OpenAI Chat in to any OpenAI-compatible host. Translation only on failover, behind conformance tests. |
| "Best model" changes monthly and benchmarks disagree | A catalog entry per model: tier, prices, cache-read multiplier, data policy, source URL, as-of date. Tiers come from your own accept rate first, public benchmarks second. |
| Counterfactual cost on another provider is a guess (tokenizers and cache rules differ) | The ledger labels counterfactuals as estimates with a range. Real numbers come only from shadow mode and `bench` replay. |
| Laya/Jev in the routing path is premature | Core uses rules plus an escalation cascade. Laya/Jev plugs in behind a scorer interface in P2 and turns on by default only if `bench` shows it beats rules. |
| Silent quality regressions destroy trust | Shadow mode, `why <id>`, one-key bypass and fail-open ship in the first gateway release. |
| Incumbents already route across providers and do cache-aware routing | Differentiator: a gateway-agnostic, verified ledger of actual vs counterfactual cost per accepted task, across harnesses. It reads their logs too. |

## Why delegation can still win (and the trap to avoid)

In the public benchmark, 14 small dispatches cost about $11 in planner tokens and made the split setup 3x solo. The design avoids that trap three ways:

1. **Plan once into a few big bundles.** Each bundle carries goal, files in scope, constraints, verify commands and done criteria. Target 3-6 dispatches per feature, not 14.
2. **Inferencia runs the verify commands.** Workers never self-certify. A test run costs no frontier tokens.
3. **The strongest model reviews the diff plus test evidence only**, and only when risk is high or tests are missing.

Planner and workers can run on any provider and in any harness: Claude Code, Codex, Gemini CLI or a direct API call. The ledger tags every session solo or delegated and reports cost per accepted task with planner overhead included. If delegated does not beat solo on your own tasks, Mode B is cut.

## Commands

| Command | Does | Ships |
| --- | --- | --- |
| `measure` | Reads Claude Code, Codex, OpenCode and Command Code logs plus provider and gateway usage exports; logged dollars where a harness records them, catalog prices elsewhere: spend by provider and model, cache share, solo vs delegated | Week 1 |
| `catalog` | Lists models with tier, prices, cache-read multiplier, data policy, source and as-of date; edited in YAML | Week 2 |
| `savings` | Actual vs counterfactual cost on other providers' models, cache-miss cost included, labeled as an estimate | Week 2 |
| `why <id>` | The rule, inputs and alternatives behind a route | Gateway v0 |
| `off` / `on` | One-key bypass; gateway down means the harness talks direct | Gateway v0 |
| `status` | Providers, limits left, today's spend per provider, last 10 routes | Gateway v0 |
| `deals` | Catalog price changes on any provider, notice only, never auto-switch | Later |
| `bench` | Replay recorded sessions against routing policies and models | Later |

One Go binary, SQLite ledger, YAML config, no UI. A read-only dashboard page served from the binary is optional and last.

## Routing policy (no LLM call in the routing path)

1. **Classify the call, not the task.** Role comes from request shape, the same way for every wire format: title or summary call, compaction, subagent spawn, main loop, plus token count and tools or images present.
2. **Main loop stays on the user's chosen model.** Sticky per session on any provider, cache preserved. The session key is the client's session_id, else a hash of the first system and first user message. It expires after 10 minutes idle, as [OpenRouter does it](https://openrouter.ai/docs/features/prompt-caching).
3. **Background and subagent calls** go to the cheapest model at or above the role's quality floor, on any allowed provider. Same wire format first; cross-format only when the saving beats the translation risk.
4. **Escalation cascade.** On a tool error, repeated failing tests or user rejection, upgrade the tier for the rest of the session. The next tier can be on another provider.
5. **Failover.** On 429, quota or outage, move to the best equal-tier model on any provider, translate only if the format differs, log the cache loss. A circuit breaker on live traffic cools a failing deployment after 3 failures, beside the 30 s watchdog ([LiteLLM's defaults](https://docs.litellm.ai/docs/routing)).
6. **Data-policy guard.** Each model carries `zdr`, `no-train`, `trains` or `unknown`. Sensitive projects never reach `trains` or `unknown`.
7. **Budget guard.** Spend caps per day, per project and per provider. At a cap, drop one tier or stop, as the config says.

Effective price = (uncached input x input price + cached input x input price x cache-read multiplier + cache writes x input price x cache-write multiplier + output with reasoning x output price) x (1 + measured retry rate) + cache-miss penalty on a switch. Every price and multiplier comes from the catalog, per provider. None is hard-coded, and a model with no price fails loud instead of counting as $0.

## Best model possible: quality floor first, then price

Inferencia never picks the cheapest model. It picks the cheapest model that clears the quality floor for the call's role, and escalates when the floor was wrong.

- **Tiers.** frontier, strong, fast, local. Each catalog model has one. Your own accept rate per role sets it once the ledger has 20+ samples; public benchmarks set it before that.
- **Floors per role, in YAML.** Main loop: the user's choice. Review: frontier. Build bundles: strong. Title, summary and compaction: fast or local.
- **Scorer plugin (P2).** Laya or Jev as a fast System-1 scorer that predicts the floor from request features. No LLM call in the routing path. On by default only if it beats rules on `bench`.
- **Up, not only down.** On an outage, failover picks the best equal-tier model left, not the cheapest one.

## Subscriptions: max out the $20 plans, never proxy them

A flat plan is the cheapest token you can buy, but only if you use its full quota. Inferencia fills plans first and spills to per-token APIs only when quota runs out. It does this by sending whole tasks to each plan's own official client, never by passing plan credentials through the gateway.

1. **Quota tracker.** For each plan: window, limit, used, reset time. Read from harness logs or the plan's usage page. How each plan reports this is not verified yet.
2. **Value report (`subs`, P0).** API-equivalent value = tokens used under the plan x catalog per-token price. Example output: "ChatGPT Plus, September: $20 paid, $X of API-equivalent tokens, 64% of quota used."
3. **Subscription-first dispatch (P2).** The delegate engine sends each bundle to a plan with headroom whose models clear the floor, through that plan's own client: `claude -p`, `codex exec`, `opencode run`, Command Code (headless mode not verified). When every plan is near its limit, the bundle goes to per-token routing.
4. **Rules.** Official clients only. Your own plans only. Read each plan's usage policy before automating it, because automation and fair-use limits differ. No plan credential or OAuth token ever passes through inferencia.

## Gates: measured, not felt

A gate passes on a paired comparison with a confidence interval. Twenty tasks cannot prove quality parity; they can only show a large gap.

- **Grader.** Hidden tests and the verify commands decide accept or reject. An LLM judge grades only what tests cannot check: binary pass/fail, from another model family, validated on your own labels with Cohen's kappa ([Hamel Husain](https://hamel.dev/blog/posts/evals-faq/), [Eugene Yan](https://eugeneyan.com/writing/llm-evaluators/)).
- **Design.** Paired: every task runs in every arm. Report a 95% confidence interval, and cluster errors by repo, because naive errors can be more than 3x too small ([Anthropic](https://www.anthropic.com/research/statistical-approach-to-model-evals)).
- **Error analysis first.** Read failed runs before you add a metric ([Anthropic](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)).
- **Shadow mode cannot measure accept rate,** because nobody accepts or rejects a shadow answer. Use offline replay plus the verify commands.

Paired tasks needed to claim the accept rate drops by no more than the margin (one-sided alpha 0.05, power 0.8, accept rate near 70%; the research agent's calculation, a floor once errors are clustered):

| Margin | Tasks if 20% of pairs disagree | Tasks if 30% disagree |
| --- | --- | --- |
| 10 points | 124 | 185 |
| 5 points | 495 | 742 |

Fifty paired tasks only rule out a drop of 16-19 points. So Gate 1 runs in two steps. First, a 30-task pilot measures the cost ratio and how often pairs disagree. The cost-ratio interval narrows fast. Then a power-sized set settles accept rate. To keep the large set cheap, run the arms under your own plans through their official clients where you can. It then costs quota, not API dollars.

## Durable execution: Temporal's semantics, a SQLite log for now

The delegate engine copies Temporal's guarantees into a SQLite event log inside the one binary. Running Temporal itself would add a server, a UI, Postgres and Elasticsearch to a single-binary Pi deployment.

- **What we copy.** Each step (plan, worker, verify, review) is an idempotent row keyed by job, step and attempt. A step's result is stored before the next step starts, so a crash or a resume never spends tokens twice. That is the main reason Temporal runs each LLM call as an Activity ([Temporal + OpenAI Agents SDK, GA 2026-03-23](https://temporal.io/blog/announcing-openai-agents-sdk-integration)). Retries use backoff. A human approval is a waiting state, like Temporal's Signal pattern ([Temporal AI patterns](https://docs.temporal.io/ai)).
- **Why not Temporal now.** AI integrations are Python and TypeScript first; Go has only the Google ADK integration. Self-hosting means running the server, UI, Postgres and Elasticsearch, and owning schema migrations ([deployment guide](https://docs.temporal.io/self-hosted-guide/deployment)). Workflow code must also be deterministic. Temporal Cloud costs $50 per 1M Actions, down to $25 at volume ([pricing](https://temporal.io/pricing)).
- **Revisit when** workers run on several machines, or human waits last days. [DBOS Go](https://docs.dbos.dev/golang/programming-guide) is the library-only middle road, but it needs Postgres.

## Security and enterprise: threat, control, phase

Every threat gets one control and one test, and each control ships no later than the phase that opens its risk. Threat names follow the [OWASP Top 10 for LLM Applications, 2025](https://genai.owasp.org/llm-top-10/).

| Threat | Control in inferencia | Ships |
| --- | --- | --- |
| Prompt injection through tool results (LLM01) | Routing reads request shape only. Text inside messages never changes the route, headers or upstream URL. | P1 |
| The lethal trifecta in delegated workers: private data, untrusted content, a way out ([Willison](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/)) | Verify commands come from a repo allowlist, never from model output. They run sandboxed, with no secrets and no network. | P2 |
| Sensitive data disclosure (LLM02) | Data-policy guard; per-key ZDR, `store=false` and region pin ([OpenAI keeps Responses state 30+ days by default](https://developers.openai.com/api/docs/guides/your-data)); OpenRouter `provider.zdr` per request ([docs](https://openrouter.ai/docs/features/zdr)) | P1, per-key policy P3 |
| Unbounded consumption, a cost attack (LLM10) | Budgets per key, project and provider. A model with no price fails loud; some gateways count it as $0 ([Portkey](https://portkey.ai/docs/product/ai-gateway/virtual-keys/budget-limits)). | P1 |
| Fail-open as a bypass | Fail-open means falling back to a default provider. It never skips auth or budgets. | P1 |
| Credential passthrough | Provider keys stay server-side. Inferencia never accepts or forwards a token issued for another service, the same rule the [MCP security spec](https://modelcontextprotocol.io/specification/2025-06-18/basic/security_best_practices) sets. | P1 |
| SSRF through upstream URLs | Base URLs cannot point at private ranges unless the config allows it. | P1 |
| Logs that leak | Keys masked (the canonical log already shows only the last 8 characters). No prompt text logged by default. | P-1 |
| PII in prompts | Optional redaction. It must be deterministic, or it breaks prompt caching ([Kong](https://developer.konghq.com/ai-gateway/)). | P3 |
| Supply chain (LLM03) | govulncheck in CI, an SBOM, and signed releases with [SLSA](https://slsa.dev/spec/v1.0/levels) provenance from GitHub attestations | P3 |
| Enterprise asks: SSO, RBAC, per-team budgets, chargeback, audit log | Built only when a team user asks. No primary source checked yet. | P3 |

## Code shape: Rob Pike's rules, applied

One interface and four types carry the routing core. Everything else is a function or a concrete struct ([Go Proverbs](https://go-proverbs.github.io/): the bigger the interface, the weaker the abstraction).

```go
// Provider is the only interface in the routing core.
type Provider interface {
	Do(ctx context.Context, r *Request) (*Response, error)
}

type Request struct {
	Format  Format // anthropic, openai-chat, openai-responses
	Model   string
	Session string // client session_id, else a prefix hash
	Key     KeyID
	Raw     []byte // original body, forwarded unchanged when formats match
}

type Response struct {
	Provider, Model string
	Usage           Usage
	Body            io.ReadCloser // a stream or a full body
}

// Route orders candidates: the first is tried, the rest are failover.
type Route func(r *Request, c []Candidate) []Candidate
```

- **Byte-stable pass-through.** When the ingress format equals the egress format, proxy `Raw` unchanged with `httputil.ReverseProxy` and parse only the usage. Translate only across formats. This keeps `cache_control` blocks and thinking signatures intact.
- **Errors are values.** `ErrRateLimited`, `ErrQuota`, `ErrContextTooLong`; the router branches with `errors.Is` ([Go blog](https://go.dev/blog/errors-are-values)).
- **Interfaces live with the consumer;** constructors return structs. `ctx` comes first and is never stored in a struct ([Go code review comments](https://go.dev/wiki/CodeReviewComments)). Middleware stays `func(http.Handler) http.Handler`, as the repo does today.
- **Today's repo.** Replace the six-method `Backend` interface in `internal/backend/backend.go` (still used by `handler/chat.go` and `router/model.go`) with `Provider`. Fold `ChatCompletion` and `ChatCompletionStream` into one `Do`.
- **Must not exist:** ProviderFactory, PluginRegistry, a Middleware interface, a Cache interface, a Repository over SQLite, an EventBus, a workflow-engine abstraction, a Translator class tree. A Scorer stays a `func` type until Laya or Jev is a second real scorer.
- **DRY where it counts:** one canonical request, one `Usage` record, one price function. The harness adapters for `measure` are plain functions that return `[]Usage`.

## Phases and gates

| Phase | Deliverable | Gate |
| --- | --- | --- |
| P-1 (week 1) | `measure` with adapters for Claude Code, Codex CLI, OpenCode, Command Code, and usage exports from providers and gateways (Anthropic, OpenAI, Google, OpenRouter); read-only public alpha with opt-in install count | Stop if fewer than 10 outside users try it in 4 weeks and your own data shows nothing delegable or routable |
| P0 (weeks 2-3) | `catalog`, `savings` with cross-provider counterfactuals, subs value report, solo vs delegated tag, cost per accepted task: a 30-task pilot, then a power-sized paired set (see Gates), workers on at least 2 providers | **Gate 1:** a cross-provider route or delegation beats solo on cost at equal accept rate, else cut Mode B |
| P1 (weeks 3-5) | Gateway. Ingress: OpenAI Chat (exists) and Anthropic Messages. Egress: Anthropic native and OpenAI-compatible. Failover, budgets, shadow mode, `why`, `off`, fail-open, conformance tests per ingress-egress pair | Gate 1 PASS. On PROVISIONAL (cost win with a 95% upper bound under 0.9, quality sample too small), it ships in shadow mode only: routes are logged, not enforced. |
| P2 (weeks 5-6) | Delegate engine in Go, ported from cc-delegate, with subscription-first dispatch; OpenAI Responses ingress for Codex; scorer interface with Laya/Jev | **Gate 2:** beats solo on `bench` replay |
| P3 (optional) | Gemini native ingress and egress, Bedrock and Vertex egress, Laya/Jev on by default, dashboard, `deals` | Only with evidence that rules leave money behind |

Current repo reality: 4.0k non-test Go lines and one ingress (`/v1/chat/completions`). That is the OpenAI-compatible format most hosts speak, so it is the right first door. No Anthropic or Responses ingress, and no cost, catalog or failover logic. MLX and TTS backends should be split out of the core binary.

## Open risks

- **Addressable spend may be small for other users.** Your data is one person across four harnesses. P-1 exists to find out before building.
- **Wire-format fidelity per provider.** Anthropic thinking-block signatures, Gemini thought signatures in multi-turn tool calls, OpenAI reasoning items in Responses, partial tool JSON and streamed usage are where gateways break. Each ingress-egress pair gets its own conformance tests.
- **Counterfactuals are estimates.** Tokenizers, cache rules and reasoning-token billing differ by provider. The ledger shows a range, not one number.
- **Incumbents ship cross-provider routing and ledgers.** Mitigation: the ledger works with any gateway's logs.
- **Scope vs kill date.** Many providers on the same 2026-11-11 date. Mitigation: one OpenAI-compatible egress covers most hosts; Gemini native and Responses ingress wait for P2-P3.
- **Attention.** Solo project next to Playlotron and the pet feeder. The kill date is the guard.
- **Unverified:** DeepSeek cache pricing; cache-read multipliers and minimum prefix sizes for OpenAI, Gemini and DeepSeek; OpenAI's and Google's policies on routing subscription traffic; whether Gemini CLI accepts a base-URL override and how it exposes token usage; whether OpenCode Go bills per token or flat (if flat, its logged cost is notional); Cursor and aider-desk log formats (not checked); the share of spend that is chatter for other users.

## Sources

Anthropic and policy claims below were found by a research sub-agent and not all re-opened by me; treat as reported.

- [RouteLLM: cost-effective LLM routing](https://lmsys.org/blog/2024-07-01-routellm/)
- [Planner + executor benchmark, akitaonrails](https://akitaonrails.com/en/2026/04/25/llm-benchmarks-vale-a-pena-misturar-2-modelos/)
- [Claude Code environment variables](https://code.claude.com/docs/en/env-vars)
- [Anthropic prompt caching](https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching)
- [Anthropic third-party harness terms, Gigazine](https://gigazine.net/gsc_news/en/20260220-anthropic-third-party-block)
- [Jev, TypeSafe System One](https://docs.typesafe.ai/introduction)
- [Laya, open System 1 decision engine](https://github.com/NandhaKishorM/laya)
- Local: codex-router tool-result aging benchmark (`docs/tool-result-aging-benchmark.md`), inferencia repo survey, own transcript analysis of 2026-09-30.
- Local, read 2026-10-17: Codex CLI session logs (`~/.codex/sessions`, 48 files, 8 with token counts), Codex config (traffic goes through codex-router on the Responses wire format), Gemini CLI chat files (`~/.gemini/tmp`, 16 files, no token counts).
- Research sub-agent, 2026-10-17: Temporal, gateway practice (Portkey, LiteLLM, OpenRouter, Vercel, Cloudflare, Envoy), eval gates, OWASP LLM 2025 and Go design. Each claim links to its opened page inline in Gates, Durable execution, Security and Code shape.
