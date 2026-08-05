# Observability Platform — Project Context

This file gives you full context on what we're building, why, and every decision made so far. Read this before starting any task.

---

## What we're building

A unified observability platform covering **AI/LLM agents** and **normal systems** (APIs, frontend, infrastructure) in one tool. Self-hostable, open source, built for other developers.

### The core problem we're solving

Only 11% of AI agents make it to production. They fail silently — latency looks fine, error rate is 0%, but the agent is producing wrong outputs, getting stuck in loops, burning tokens on nothing useful. Existing tools (Datadog, Langfuse, LangSmith) can't catch this because they measure infrastructure or log traces — they don't measure reasoning quality.

### Our differentiator

We capture what nobody else does:
- **Decision traces** — every step in an agent loop: think → tool_call → tool_result → replan
- **Loop detection** — SHA256 fingerprint of `tool_name + input` per step; duplicate fingerprints in same run = loop
- **Tool analytics** — success rate, p95 latency, failure reasons per tool
- **Token efficiency** — tokens per step correlated with eval scores; spikes = reasoning degradation
- **Eval scores** — LLM-as-judge quality scores (correctness, completeness, efficiency, safety) attached to every run
- **Termination reason** — explicit: `clean | max_steps_reached | context_limit | error | loop_detected | timeout`

This makes us the first tool that answers: "Why did my agent fail, and exactly where did it go wrong?"

---

## Tech stack

| Layer | Choice | Reason |
|---|---|---|
| Ingest API | Go | Low overhead, high-throughput, same language as Prometheus/Vector |
| Storage | TimescaleDB (Postgres + extension) | Already know Postgres, simpler ops, fast enough for MVP scale. No ClickHouse until we actually need it. |
| Dashboard | React + TypeScript + Recharts | Standard, fast to build |
| Collectors | Go SDK + JS snippet + Go infra agent | One per signal type |
| Packaging | Docker Compose | One-command local setup for OSS users |
| Cloud deploy | Fly.io or Railway | Hosted trial experience |

### Philosophy on tech choices
Ship fast, solve scaling problems when they're real. TimescaleDB over ClickHouse for now — if we need ClickHouse later, we'll have paying users and a clear migration path. That's a good problem to have.

---

## Schema

File: `schema.sql` — 11 tables + 7 views total.

### Core tables
```
projects          — multi-tenant root, every row scoped to project_id
api_keys          — hashed, project-scoped, never store plaintext
ai_traces         — one row per LLM call (tokens, cost, latency, status)
api_events        — one row per HTTP request (method, path, status, duration breakdown)
infra_metrics     — time-series name/value rows per host
alert_rules       — threshold alerts with optional service/path/model scope
alert_events      — audit log of every alert fired
```

### Agent observability tables (the differentiator)
```
agent_runs        — root record per agent invocation
                    key cols: status, termination_reason, loop_detected,
                              loop_step_index, total_tokens, total_cost_usd
agent_steps       — one row per step in the agent loop
                    key cols: step_type, tool_name, tool_input, tool_success,
                              input_fingerprint (SHA256 for loop detection)
agent_tools       — auto-populated tool registry, powers tool analytics
agent_evaluations — quality scores per run: correctness, completeness,
                    efficiency, safety (0.0–1.0 each)
```

### Key views
```
v_api_error_rate_1h          — error rate per service, last hour
v_api_latency_percentiles_1h — p50/p95/p99 per endpoint, last hour
v_ai_cost_24h                — token usage + cost per model, last 24h
v_agent_tool_stats           — success rate + p95 latency per tool
v_agent_run_health           — completion rate, loop rate, avg cost per agent
v_agent_loops                — runs with repeated fingerprints (actual loops)
v_agent_efficiency           — tokens/step correlated with eval scores
```

### Hypertables (TimescaleDB)
All time-series tables use `create_hypertable()` on `timestamp`:
`ai_traces`, `api_events`, `infra_metrics`, `agent_runs`, `agent_steps`

---

## MVP phases

Legend: `[x]` done · `[~]` in progress · `[ ]` pending

### Phase 1 — Foundation (weeks 1–2)
- [x] Docker Compose: TimescaleDB pg18
- [x] Go ingest API skeleton: chi router, pgx/v5 pool, zap logging, env config
- [x] Migration tooling: golang-migrate CLI + Makefile targets
- [x] Migration 000001: `projects` + `api_keys` (uuidv7 PK, FK cascade)
- [x] Store layer: `Projects` + `APIKeys` repos w/ interfaces
- [x] API key auth: `internal/auth` sha256 hash, `apiKeyAuth` middleware injects project_id into ctx
- [x] Admin CRUD: `/v1/admin/projects` + `/v1/admin/projects/{id}/keys` + `/v1/admin/keys/{id}`
- [x] Shared helpers: `errors.go` + `json.go` (1 MiB body cap, validator/v10)
- [x] OpenAPI spec + Scalar UI at `/v1/docs` (swag + air hot reload)
- [x] Migration 000002 + 000003: ingest tables (`ai_traces`, `api_events`, `infra_metrics`) and agent tables (`agent_runs`, `agent_steps`, `agent_tools`, `agent_evaluations`); hypertables via `WITH (tsdb.hypertable)` with per-table `segmentby` tuning
- [x] Store layer for 7 ingest tables: Insert + key queries (`Finish`, `CountFingerprint`, `UpsertSeen`, `ListByRun`)
- [x] Ingest handlers: `POST /v1/ingest/ai`, `/events`, `/metrics`, `/agent/runs`, `/agent/runs/{id}/finish`, `/agent/steps` — all gated by `apiKeyAuth`
- [x] Server-side SHA-256 fingerprint on agent steps (loop detection input)
- [x] Best-effort `agent_tools` registry upsert in goroutine off the step hot path
- [x] Response shape unified: `jsonResponse` for bodies, `noContentResponse` for 204
- [x] Seed script: `cmd/migrate/seed` creates dev project + key (`make seed` prints plaintext)
- [x] Batch buffer (`internal/batch`): per-table `Buffer[T]` with channel queue + ticker + flush callback; ai_traces, api_events, infra_metrics, agent_steps move to `pgx.CopyFrom`. agent_runs stays sync (Finish UPDATE would race a buffered insert). Handler generates `uuid.NewV7()` to preserve the `{id,timestamp}` 201 contract. Buffer-full → 503 + `Retry-After: 1`
- [x] Graceful shutdown: `signal.NotifyContext` (SIGINT/SIGTERM) → `srv.Shutdown` → `batchers.Stop` → `pool.Close`, all under one `SHUTDOWN_TIMEOUT` budget
- [ ] Real admin auth (replace unprotected `/v1/admin/*`)
- [x] Rate limit on `/v1/ingest/*` (`httprate`): per-IP layer before `apiKeyAuth`, per-API-key layer after; both route 429 through `rateLimitResponse`

### Phase 2 — Collectors (weeks 3–4)
- [ ] Go SDK: LLM call wrapper (OpenAI + Anthropic), HTTP middleware for API metrics
- [ ] Go infra agent: CPU/memory/disk polling every 10s
- [ ] JS browser snippet: page views, JS errors, Core Web Vitals via `PerformanceObserver`
- [ ] README + quickstart docs for each collector

### Phase 3 — Dashboard (weeks 5–6)
- [ ] React + TypeScript dashboard
- [ ] Views: AI traces, API metrics (p50/p95/p99 charts), infra per host, agent run explorer
- [ ] Agent-specific views: decision trace timeline, tool heatmap, loop flagging
- [ ] Go query API serving TimescaleDB queries to dashboard

### Phase 4 — Alerting + multi-project (week 7)
- [ ] Threshold alerts on error rate, p99 latency, LLM cost spike, agent loop rate
- [ ] Webhook + email delivery
- [ ] Multi-project isolation (already in schema via project_id)

### Phase 5 — Ship (week 8)
- [ ] Single `docker-compose.yml` for self-host
- Landing page + docs (what it does, quickstart, architecture)
- GitHub release, MIT license
- Hosted trial on Fly.io or Railway

### Post-MVP (v1.1+)
- OpenTelemetry ingestion (OTLP endpoint)
- LLM-as-judge automated eval pipeline
- SSO
- Stripe billing for cloud tier

---

## What to build next

Phase 1 follow-ups:
1. Real admin auth — deferred; safety gating only, not differentiator. Unprotected `/v1/admin/*` ok for dev.

Phase 2 (in flight):
- [x] 2.1 Agent inspector query API (`/v1/agent/runs`, `/{runID}`, `/{runID}/steps`, `/{runID}/loops`) — TDD-driven, real-Postgres tests
- [ ] 2.2 Python SDK (`keelwave-python`)
- [ ] 2.3 Demo agent (looping + clean)
- [ ] 2.4 TypeScript SDK (`keelwave-ts`)
- [ ] 2.5 OTLP ingest endpoint (`/v1/otlp/traces` w/ OpenInference)
- [ ] 2.6 Dashboard MVP (React + Recharts) + view rollups + cursor pagination

### Folder structure (actual)
```
cmd/
  api/            — main.go, api.go (router), context, errors, json, middleware,
                    docs (Scalar UI), health, projects, keys, query (params),
                    ingest (shared DTO),
                    ai_traces, api_events, infra_metrics, agent
  migrate/
    migrations/   — golang-migrate up/down SQL pairs (000001..000003)
web/              — React + TypeScript dashboard (future)
docs/             — swag-generated OpenAPI package + handwritten notes/
internal/
  auth/           — API key Generate / Hash / Parse
  db/             — pgx/v5 pool setup
  env/            — env-var loader
  store/          — storage.go + projects + apikeys +
                    ai_traces + api_events + infra_metrics +
                    agent_runs + agent_steps + agent_tools + agent_evaluations
.air.toml         — hot reload + `make gen-docs` pre_cmd
docker-compose.yml
Makefile
CLAUDE.md
```

Pending packages (added as work lands): `internal/batch`, `internal/ingest` (or keep handlers in `cmd/api`).

---

## SDK design philosophy — zero friction for devs

Developer tools die from integration friction, not missing features. If it takes more than 5 minutes to get data flowing, devs won't bother. The model to beat is Helicone — two lines for LLM cost tracking. We match or beat that for every signal.

**The rule: zero config to get started, opt-in config to get more. API key is the only required thing.**

Everything else — service name, environment, custom tags — is optional and inferred where possible.

### AI traces — one line

```go
// before
resp, err := client.Chat(ctx, req)

// after
resp, err := obs.Trace(client).Chat(ctx, req)
```

Wrapper captures model, tokens, cost, latency, status automatically. No manual instrumentation.

### API metrics — one line middleware

```go
r.Use(obs.Middleware())
```

Captures every request automatically. Path normalization built in — `/users/123` → `/users/:id` without dev doing anything.

### JS frontend — one script tag

```html
<script src="https://cdn.yourtool.dev/obs.js" data-key="YOUR_KEY"></script>
```

Auto-tracks page views, JS errors, Core Web Vitals. Zero config.

### Infra agent — one command

```bash
curl -sSL https://yourtool.dev/install | OBS_KEY=xxx bash
```

Runs as background process, starts sending CPU/memory/disk immediately.

### Agent tracing — one wrapper

```go
run := obs.NewRun(ctx, "research-agent")
defer run.Finish()

run.Step("think", thought)
run.ToolCall("search", input, output, err)
run.ToolCall("calculator", input, output, err)
```

Loop detection, fingerprinting, cost rollup — all automatic inside the SDK. Dev just marks steps.

---

## Coding conventions

- Go: standard library where possible, `pgx/v5` for Postgres, `chi` or `net/http` for routing
- No ORM — raw SQL, queries live in `internal/db/`
- Errors returned, not panicked
- Every handler validates input before touching the DB
- `project_id` comes from the API key lookup in middleware, never from the request body
