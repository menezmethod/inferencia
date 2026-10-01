# Adaptive Inference Runtime — north star

> Status: 2026-10-01. This document is the specification. The table below maps
> what exists in the repo today to it, so the gap is always explicit.

## Where we are

| Piece | State | Where |
|---|---|---|
| Explicit mode (faithful proxy, `backend/model`, virtual roles) | **done** | `internal/routing`, `internal/handler/chat.go` |
| Multi-provider gateway (OpenAI-compatible adapters, streaming, health, load balancing) | **done** | `internal/backend`, `internal/watchdog` |
| Auth, rate limiting, metrics, structured logs | **done** | `internal/middleware` |
| Auto mode + policies (`auto`, `auto:quality`, `auto:balanced`, `auto:economy`, `auto:free`) | **done** | `internal/handler/auto.go` |
| Jev/Laya semantic layer (tier judgment, `/v1/systemone` passthrough) | **done** for tier selection | `internal/decision` |
| Confidence gate / abstention → fallback | **done** | `decision.Pick`, `planAuto` |
| Decision records (reason codes, log line, metrics) | **partial** — no durable outcome store yet | `X-Routed-Reason`, `auto decision` log |
| Conversation affinity | **not built** | — |
| Model intelligence registry (pricing, quotas, capabilities, P(success)) | **not built** — needs telemetry first | — |
| Planner over execution strategies (cascade, maker/checker, verify, escalate) | **not built** — deliberate: harnesses own multi-step orchestration until asked otherwise | — |
| Budget manager (credits, spend envelopes, degrade-not-fail) | **not built** | — |
| Verification runtime | **not built** | — |
| Learning loop (store outcomes → update statistics) | **not built** — starts with decision/outcome capture | — |

Next trigger: once decision records and outcome labels are captured, build the
registry and the budget-aware planner; verification strategies only if the
harness-facing contract changes.

---

# One-liner

An always-on, OpenAI-compatible inference runtime that proxies requests across local and cloud models, handles provider limits/failures/budgets automatically, and optionally uses Jev/Laya to dynamically assemble the lowest-cost execution path capable of producing frontier-quality output.

The system has two fundamentally different ways to operate:

```text
EXPLICIT MODE
User chooses model
      ↓
proxy request as faithfully as possible
      ↓
handle auth / limits / retries / provider translation
      ↓
chosen model


AUTO MODE
User chooses "auto"
      ↓
understand task
      ↓
determine required capability
      ↓
inspect available resources/models/budget
      ↓
choose execution strategy
      ↓
run / verify / escalate as necessary
      ↓
best attainable response
```

The app is therefore **both an inference gateway and an adaptive inference planner**.

---

# Core principle

The runtime should optimize for:

```text
Produce the best attainable response
with the resources currently available.

Among plans likely to produce comparable quality,
choose the cheapest one.
```

Quality comes first.

Cost optimization means:

```text
don't spend more than necessary
```

not:

```text
always use the cheapest model
```

A $0.002 model that repeatedly fails is not cheaper than a $0.01 model that succeeds immediately.

---

# Explicit model mode

Users must always be able to bypass automatic selection.

Examples:

```json
{
  "model": "openai/gpt-x"
}
```

```json
{
  "model": "anthropic/claude-x"
}
```

```json
{
  "model": "local/qwen"
}
```

The runtime should then behave like a normal inference proxy.

It should not silently replace the requested model because Jev/Laya thinks something else is better.

It may still perform infrastructure-level operations necessary to keep the request working, such as provider translation, credential management, rate-limit handling, streaming normalization, retries against the same deployment/model where appropriate, context validation, telemetry, caching and provider-specific compatibility fixes.

Whether cross-model fallback is allowed after the explicitly requested model becomes unavailable should be an explicit policy such as:

```text
strict
same-model-fallback
compatible-fallback
```

`strict` means the user's chosen model really means that model.

---

# Auto modes

Auto means:

```json
{
  "model": "auto"
}
```

But there should eventually be several policies:

```text
auto
auto:quality
auto:balanced
auto:economy
auto:free
```

`auto` can default to balanced behavior.

`auto:quality` aggressively spends where better intelligence has meaningful expected value.

`auto:balanced` seeks near-frontier output while avoiding unnecessary frontier cost.

`auto:economy` aggressively uses inexpensive models, cascades and verification.

`auto:free` only uses currently free or local resources unless the user explicitly changes policy.

These are policies over the same planner, not separate routing implementations.

---

# Request path

The complete request path should be:

```text
CLIENT
  │
  ▼
OPENAI-COMPATIBLE API
  │
  ▼
REQUEST NORMALIZER
  │
  ├── explicit model ───────────────► normal proxy path
  │
  └── auto
        │
        ▼
HARD REQUIREMENTS
        │
        │ context length
        │ modalities
        │ required tools
        │ structured output
        │ privacy / region
        │ provider restrictions
        │ user budget
        │ remaining credits
        │ latency requirements
        ▼
JEV / LAYA
semantic assessment
        │
        │ task family
        │ capability requirement
        │ difficulty
        │ ambiguity
        │ reasoning requirement
        │ verifiability
        │ maker/checker usefulness
        │ confidence
        ▼
EXECUTION PLANNER
        │
        ▼
MODEL INTELLIGENCE REGISTRY
        │
        │ model capabilities
        │ current pricing
        │ free quotas
        │ rate limits
        │ provider health
        │ latency
        │ public benchmarks
        │ internal evals
        │ production outcomes
        ▼
P(success | request, model/strategy)
        │
        ▼
PLAN OPTIMIZER
        │
        ▼
GATEWAY / EXECUTOR
        │
        ▼
VERIFY / ESCALATE IF APPROPRIATE
        │
        ▼
RESPONSE
        │
        ▼
TELEMETRY + LEARNING
```

---

# Jev and Laya

Jev/Laya should provide **semantic judgment**, not facts the runtime already knows.

They answer things such as:

```text
What kind of task is this?

How difficult does successful completion appear?

Does it require unusually strong reasoning?

Does it appear ambiguous?

Would an independent verifier likely add value?

What minimum capability class does this appear to require?
```

They should NOT decide:

```text
which provider is down
which model is cheaper
remaining account balance
context-window arithmetic
token counts
current model pricing
whether a free quota remains
whether an endpoint supports vision
```

Those are deterministic runtime facts.

The basic division is:

```text
Jev/Laya = fuzzy semantic understanding

Code = policy, economics and constraints
```

---

# Jev + Laya strategy

Use Laya as the normal local hot-path assessor.

It is cheap, local and trainable on our eventual workload.

Use Jev as an optional stronger external decision surface for uncertain cases, bootstrapping and auditing.

Conceptually:

```text
request
   ↓
 Laya
   ↓
confident?
 /      \
yes      no
 │        │
 │       Jev
 │        │
 └───┬────┘
     ▼
 planner
```

Also periodically send a small sample of Laya decisions to Jev in shadow mode.

Cases where:

```text
Laya != Jev
```

are valuable evaluation/training examples.

Eventually our real production outcomes matter more than either one's generic assumptions.

---

# Model tiers are requirements, not static models

Jev's `nano / fast / balanced / frontier / reasoning` concept is useful, but never hard-code:

```text
frontier = Claude X
balanced = GPT Y
```

Instead think:

```text
economy
standard
advanced
frontier
specialized
```

as approximate capability requirements.

Then resolve them dynamically.

Example:

```text
task requires ADVANCED

registry:

Model A   success .96   $0.025
Model B   success .95   $0.009
Model C   success .92   FREE
Model D   success .97   $0.080
```

If the required quality floor is `.94`:

```text
Model B wins.
```

Tomorrow a new free model scores `.95`.

It immediately becomes eligible without retraining Jev or Laya.

---

# Do not only choose models

The planner should choose an **execution plan**.

Sometimes the optimal plan is:

```text
frontier model once
```

Sometimes:

```text
free model
→ verify
→ return
```

Sometimes:

```text
cheap model
→ verify
→ stronger model only if needed
```

Sometimes:

```text
cheap maker
→ independent checker
```

Sometimes:

```text
two cheap candidates
→ deterministic test
→ best valid result
```

Therefore candidates eventually look more like:

```text
DIRECT(model)

CASCADE(modelA → modelB → frontier)

MAKER_CHECKER(modelA, modelB)

FREE_THEN_ESCALATE(free, paid)

FRONTIER_DIRECT(model)

LOCAL_ONLY(model)
```

The optimizer selects the cheapest strategy expected to meet the required quality.

That is much more powerful than ordinary model routing.

---

# Verification

Verification is one of the most important pieces of the runtime because it allows cheap models to compete safely.

Examples:

```text
code         → compile / tests / lint
JSON         → schema validation
SQL          → execute / explain / expected result
math         → deterministic calculator
tool call    → inspect actual tool result
extraction   → schema / expected fields
research     → citation/source validation
```

If the cheap model passes a reliable validator:

```text
stop.
```

Do not call a frontier model merely because one was available.

If verification fails:

```text
escalate.
```

This converts intelligence into something the runtime can purchase incrementally.

---

# Budget and credits

Credits are runtime resources, not a binary permission flag.

The planner receives:

```text
credits remaining
percentage remaining
free provider quotas
local compute availability
per-request cost ceiling
account spending policy
```

As credits decrease, the available execution strategy changes continuously.

```text
healthy budget
    ↓
normal planner behavior

lower budget
    ↓
favor cheaper first attempts
use verification/cascades more
avoid unnecessary parallel calls

critical budget
    ↓
free/local first
save paid intelligence for tasks where it matters

zero paid credits
    ↓
free APIs
local inference
cached results
deterministic tools
whatever viable resources remain
```

The runtime should not produce:

```text
insufficient credits
```

while it still has a viable inference path.

Instead:

```text
budget exhaustion degrades strategy,
not service availability.
```

Only when **no viable execution resource remains** should inference truly fail.

---

# Budget preservation

Remaining balance should influence how freely the runtime spends.

If a user has $1 left:

```text
"rewrite this heading"
```

should consume almost none of it.

That dollar may be much more useful for:

```text
"my production deployment is broken"
```

So task importance/difficulty and remaining resources jointly determine an appropriate spend envelope.

The user can override this completely:

```text
"use the best available model"
```

or:

```text
"free only"
```

---

# Model intelligence registry

Maintain a continuously updated local registry containing every model we can actually invoke.

Each entry should track:

```text
provider
model id
family/version
active status

input/output pricing
free quota/status
cached-input pricing

context window
max output
text/image/audio/etc
tools
structured output
reasoning support

latency
throughput
failure rate
429 rate
current health

external benchmark priors

our own:
task success rates
difficulty success rates
verification success
cost per successful task
fallback frequency
latency distribution
sample count
```

Sources can include provider APIs, OpenRouter/OpenCode-compatible catalogs, external benchmark services and our own telemetry.

Public benchmarks bootstrap new models.

Our own performance data eventually dominates.

---

# New models

A newly released model should be usable quickly.

```text
discover model
    ↓
add registry entry
    ↓
import capabilities/pricing
    ↓
external benchmarks = initial prior
    ↓
run internal eval suite
    ↓
shadow traffic
    ↓
small canary
    ↓
normal candidate pool
```

No Jev/Laya retraining should be required merely because the market changed.

---

# Quality estimation

The important number eventually becomes:

```text
P(success | request, model)
```

and later:

```text
P(success | request, execution strategy)
```

At first this can be simple tables:

```text
model × task × difficulty
```

Over time train a predictor using:

```text
task
difficulty
Laya/Jev outputs
prompt characteristics
context size
tools
modality
language
workflow
model
strategy
historical outcomes
```

Do not optimize generic leaderboard rank.

Optimize performance on **our users' actual work**.

---

# Real objective

The optimizer should conceptually solve:

```text
maximize expected task success

subject to:
    available resources
    hard capability requirements
    user policy
    privacy requirements
    remaining budget
    provider availability
    acceptable latency

then among plans with comparable expected quality:

    minimize expected total cost
```

Cost means more than token price.

Include:

```text
initial inference
expected output length
tool-loop calls
verification
retry probability
fallback probability
provider fees
```

The meaningful economic metric is:

```text
cost per successful task
```

not:

```text
cost per million tokens
```

---

# Provider/runtime layer

The gateway also has responsibilities completely independent of Auto.

It should normalize providers behind one API and keep things running.

That includes provider adapters, streaming normalization, auth/key management, local and cloud endpoints, health checks, rate-limit tracking, RPM/TPM limits, concurrent-request limits, context limits, retries, cooldowns, circuit breakers, failover, timeouts, usage/cost accounting, caching where appropriate and consistent errors.

The semantic planner answers:

```text
what should execute this?
```

The infrastructure layer answers:

```text
how do I successfully execute it right now?
```

Never mix those two responsibilities.

---

# Always-on behavior

The runtime should progressively recover rather than immediately fail.

```text
preferred deployment
      ↓ unavailable
same model / alternate endpoint
      ↓ unavailable
equivalent qualified model
      ↓ unavailable
cheaper/free/local alternative
      ↓
degraded but useful result
```

For explicit strict-model requests, respect the user's model-selection contract.

For Auto, resilience is part of the product.

Provider outage, exhausted quota, expired free allowance or insufficient paid credits simply change the planner's candidate graph.

---

# Conversation affinity

Do not unnecessarily switch models every message.

Keep a conversation on its current model where that still makes sense because switching can destroy provider-side caches, continuity and tool state.

Re-route when the task meaningfully changes, the current model is no longer adequate, context requirements change, the provider becomes unhealthy, budget pressure changes significantly or the current path repeatedly fails.

---

# Observability

Record every Auto decision.

Store the task assessment, hard requirements, candidate models, filtered candidates, predicted quality, predicted cost, selected strategy/model, fallbacks, actual latency, actual token usage, actual cost, verification outcome, provider errors, final quality/evaluation, Laya/Jev versions and registry/policy version.

The user-facing reason should be concise:

```text
Selected Model B:
lowest-cost available model predicted to satisfy
the requested quality and tool requirements.
```

Never expose hidden reasoning.

Use machine-readable reason codes such as:

```text
TASK_CODE_DEBUG
CAPABILITY_ADVANCED
TOOLS_REQUIRED
QUALITY_FLOOR_MET
CHEAPEST_QUALIFIED
```

---

# Learning loop

Every completed request improves the system.

```text
request
  ↓
route
  ↓
execute
  ↓
verify/evaluate
  ↓
store outcome
  ↓
update model/task statistics
  ↓
better future prediction
```

Especially prioritize learning from:

```text
Jev/Laya disagreement
low-confidence routes
fallbacks
unexpected failures
unexpected cheap-model successes
high-cost routes
new models
```

Those examples contain the most information about routing boundaries.

---

# Product interface

Keep the client simple.

Normal usage:

```json
{
  "model": "auto",
  "messages": [...]
}
```

Maximum quality:

```json
{
  "model": "auto:quality"
}
```

Economical:

```json
{
  "model": "auto:economy"
}
```

No paid inference:

```json
{
  "model": "auto:free"
}
```

Specific model:

```json
{
  "model": "anthropic/claude-whatever"
}
```

Local model:

```json
{
  "model": "local/qwen-whatever"
}
```

The same API handles all of them.

---

# What the app actually is

It is **not merely an AI router**.

It is:

```text
OpenAI-compatible inference proxy
        +
multi-provider gateway
        +
local/cloud abstraction
        +
limits/quota manager
        +
resilience layer
        +
model intelligence registry
        +
Jev/Laya semantic decision layer
        +
execution planner
        +
budget manager
        +
verification/escalation runtime
        +
continuous evaluation system
```

Explicit mode behaves like infrastructure.

Auto mode adds intelligence.

---

# The invariant

The whole system should follow this rule:

> **Never spend more intelligence than the task needs, never sacrifice meaningful output quality merely to save tokens, and never stop while another viable inference path remains.**

Or, even shorter:

> **Route explicitly when told; otherwise keep trying the cheapest viable path until you have the best response the user's available resources can buy.**
