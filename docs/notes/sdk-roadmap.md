# SDK roadmap — `keelwave-py` (and schema dependencies)

Working reference for what's in the SDK today, what's coming, and what the server schema needs to grow to support each step. Two things drive the order:

1. **Wire protocol changes (server) must land before SDK changes that depend on them.** SDK shape leans on schema; we won't ship API surface that the server can't render.
2. **Multi-language parity matters.** Anything that becomes the "primary" SDK shape must translate to TS/Go SDKs later. Python-only sugar can ship after the cross-language pattern is established.

---

## Status — v0.0 (current)

Sync + async classes, explicit context-manager + method-call API. Real Postgres tests, end-to-end happy + error + loop-detection paths.

```python
with keelwave.run("agent", input=...) as run:
    run.step("think", ...)
    run.tool_call("search", input=..., output=..., ok=True)
    run.set_output(...)

async with akeelwave.run("agent") as run:
    await run.step("think", ...)
    await run.tool_call(...)
```

Surface:
- `Keelwave` / `AsyncKeelwave` — top-level clients, share `_client.py` helpers (`raise_for_status`, `parse_retry_after`).
- `Run` / `AsyncRun` — context-manager lifecycle, auto-finish on exception (`status="failed"`, `termination_reason="error"`).
- Typed error hierarchy under `VigilError` (auth, validation, rate-limited, buffer-full, server, transport) — sync + async paths share the mapper.

Why context-manager-first and NOT decorator-first:
- Deterministic. No `contextvars` magic, no async-task ctx leakage.
- Translates 1:1 to Go, Rust, Java SDKs. Decorators don't.
- Server schema is currently flat (`agent_steps` has no `parent_step_id`); decorator-style nesting would emit IDs nobody renders.

Decorators come later, as **sugar** on top of the explicit API, after the schema can render them.

---

## v0.1 — schema-only prep for nested steps (server)

No SDK change. One migration to unlock the next sugar layer.

```sql
-- cmd/migrate/migrations/000004_step_tree.up.sql
ALTER TABLE agent_steps
  ADD COLUMN IF NOT EXISTS parent_step_id uuid,
  ADD COLUMN IF NOT EXISTS depth          int  NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS agent_steps_parent_idx
  ON agent_steps (agent_run_id, parent_step_id)
  WHERE parent_step_id IS NOT NULL;
```

Backwards compatible — `parent_step_id IS NULL` = top-level step (current behavior). Existing inserts keep working.

Server work:
- `cmd/api/agent.go` step ingest validator accepts an optional `parent_step_id`.
- `cmd/api/query.go` plus the existing `ListByRun` keep working without changes.
- New endpoint `GET /v1/agent/runs/{id}/tree?at=…` builds the tree via `WITH RECURSIVE` and returns the rooted hierarchy. Dashboard work in 2.6 consumes it.

Loop-detection fingerprint behavior unchanged — fingerprint covers `tool_name + tool_input` only, parent context doesn't enter the hash.

---

## v0.2 — `@keelwave.observe()` decorator (Python sugar)

After v0.1 ships and dashboard can render trees. Decorator becomes sugar on the explicit API:

```python
from keelwave import observe

@observe(agent_name="research-agent")   # outermost → opens a Run
def main(query: str) -> str:
    plan = plan_step(query)
    return execute(plan)

@observe()                              # nested → step under current Run
def plan_step(query): ...

@observe(as_type="generation")          # emits an ai_trace too
def call_llm(prompt): ...
```

Implementation sketch:
- `keelwave.observe()` reads a `contextvars.ContextVar` holding the "current Run + current step_id".
- Outer call w/ no parent ctx → opens a `Run` (same `__enter__`/`__exit__` as today, just driven by the wrapper).
- Inner calls → emit `step()` with `parent_step_id = <current ctx step>` and push themselves as the new current.
- Async variant uses `asyncio.Task`-bound ContextVar so `gather()` doesn't leak parent state.

Constraint: decorator is a **wrapper** around the existing `Keelwave.run()` + `Run.step()` calls. It does not bypass the public API. If a customer mixes `with run` and `@observe` in the same code, both work, parent tracking is consistent.

What it specifically does NOT do:
- Does not become the only way to instrument. The context-manager + method API remains first-class.
- Does not run instrumentation if `KEELWAVE_API_KEY` is unset (silent no-op + warning, observability never crashes the app).
- Does not auto-instrument any framework (langchain, llamaindex). That's v1.0 territory.

Server: no change. Decorator emits the same `/v1/ingest/agent/*` calls.

---

## v0.3 — `/v1/agent/runs/{id}/tree` endpoint + dashboard tree view

Server-side recursive query, dashboard panel that renders the step hierarchy collapsibly. Tree depth limits and pagination per branch determined when dashboard work starts.

This is what makes the `@observe()` decorator actually pay off — without a tree view, nested IDs are invisible.

---

## v1.0 — framework adapters (separate packages)

Each adapter is its own pip package:

| Package | Wraps |
|---|---|
| `keelwave-langchain` | LangChain callback handler — auto-emits steps for chain runs, tool calls, LLM calls |
| `keelwave-llamaindex` | LlamaIndex callback manager |
| `keelwave-crewai` | CrewAI hooks |
| `keelwave-openai` | thin wrapper around the OpenAI client that times calls + records to `ai_traces` |
| `keelwave-anthropic` | same for Anthropic |

Why separate packages:
- Users who don't use LangChain shouldn't pull it as a transitive dep.
- Each adapter pins to a framework version; pinning happens per-adapter, not in the core SDK.
- The "VigilMiddleware(agent)" pattern from the discussion lives here — the wrapper is framework-aware, not generic.

Server: no change. Adapters call the same SDK methods.

---

## v1.1+ — wire protocol parity for non-Python SDKs

Add `keelwave-js` (npm) + `keelwave-go` (go module). They mirror the v0 surface (context manager + method calls). The v0.2 decorator does NOT cross to other languages — it's Python-only sugar.

Cross-cutting requirement: any future server endpoint that the SDK depends on (e.g. tree, retry-policy hints) lands in the OpenAPI spec at `/v1/openapi.json` BEFORE any SDK consumes it. SDKs can codegen from the spec; we don't want hand-written endpoint stubs to drift.

---

## Decisions intentionally deferred

- **`step_success` + `error_message` on `agent_steps`** — would let the SDK record per-step failures cleanly. Wait until the first time someone needs to query "which step failed". Then it's one `ALTER TABLE`.
- **OTLP ingest endpoint** (`POST /v1/otlp/traces`) — accepts OpenInference-formatted spans. Inherits 15+ languages "for free". Worth doing AFTER the python/ts/go SDKs ship, so we know our schema can absorb OpenInference field-by-field without forcing a remap.
- **Sub-second step buffering on the SDK side** — server already buffers via `internal/batch` for `agent_steps`. Adding SDK-side batching would double the latency budget for no throughput gain.
- **`agent_runs.loop_detected` + `loop_step_index` auto-fill** — the columns exist on the table but nothing populates them today. The fingerprint-based loop detection lives at query time on `/v1/agent/runs/{id}/loops`, so the run row stays `loop_detected=false` even when the step trace contains loops. Three possible fixes — server scans the loop view inside the `Finish` handler and writes back, SDK polls the loop endpoint before posting `finish`, or drop the columns and derive `loop_detected` from a join on the loop view. All three depend on whether `agent_steps` has flushed from the batch buffer by the time anyone reads. Defer until dashboard work (Phase 2.6) picks the join shape — that determines which path is right.
- **Per-call cost in USD** — `agent_runs.total_cost_usd`, `agent_steps.cost_usd`, and `ai_traces.cost_usd` stay NULL today because the SDK has no price source. Options: hardcoded price table in SDK (drifts as providers change pricing); server-stored price map keyed by `model` (one source of truth, SDK sends tokens, server computes); a separate `keelwave-pricing` package. Server-side computation is the cleanest. Worth doing once we ship more than one demo agent.
- **`parseAtWindow` ±1s default is too narrow for run-scoped endpoints** — `/v1/agent/runs/{id}/steps` and `/.../loops` accept `?at=...`, but the server then builds a `[at-1s, at+1s]` window. That's fine for "show me the step at this timestamp" but wrong for "show me everything in this run" — agent runs commonly span seconds to minutes, so the window misses every step except possibly the first. Demos currently omit `at` to fall back to the 30-day default window. Real fix: when the URL is run-scoped (the `runID` is in the path), derive the window from the run row itself — `from = agent_runs.timestamp`, `to = COALESCE(finished_at, now())`. Removes the footgun entirely. Defer until /v1/agent/runs/{id}/tree (Phase 0.3) needs the same treatment; both endpoints can adopt a `runWindow(ctx, runID)` helper at the same time.
- **OTel-style architecture (client-generated IDs + bg-thread queue + ContextVar parent linkage)** — the broader OpenTelemetry pattern is built on `opentelemetry-sdk`'s `BatchSpanProcessor` for fire-and-forget telemetry: the SDK supplies its own uuidv7-like span IDs, parent/child links propagate via a ContextVar, and an internal worker thread drains a queue and ships batches over HTTP. Keelwave today goes the other way — server generates `agent_runs.id` via `DEFAULT uuidv7()`, SDK blocks on the start round-trip to learn it. The OTel model has real DX wins: zero per-call latency, no need for an async client (sync API works in async contexts because nothing actually blocks), and the `@observe()` decorator falls out almost for free because parent/child is just a ContextVar lookup. The half-step toward this pattern is already shipped — `keelwave/_context.py` carries the active Run for `wrap_anthropic` to read, and that same ContextVar can drive a future `@keelwave.observe()` decorator. Full pivot wants three things together: (1) schema-flip so `agent_runs`/`agent_steps`/`ai_traces` accept client-supplied uuidv7 (currently they don't trust client IDs), (2) bg-thread + queue for the fire-and-forget endpoints, (3) drop the round-trip wait in `Run.__enter__`. Defer until we ship dashboard + cloud trial + a second SDK and real users report SDK latency or async friction as a complaint. Premature pivot before users feel the friction = bikeshedding.

---

## Pattern to remember

> Server schema and wire protocol come first. SDK shape is a consequence, not a starting point.

Every SDK feature should answer: "what does the server need to record this and render it?" If the answer is "nothing new," we can ship the SDK feature alone. If it's "we'd need columns or endpoints," the migration ships first, the SDK consumes it later.

---

## Cross-reference

- Keelwave server: `CLAUDE.md` Phase 2 sub-list — Python SDK is item 2.2. This roadmap subsumes 2.2 with finer granularity.
- Memory: `feedback_go_comments.md` for code-style policy in the server SDK that consumes/exports the same wire shapes.
- Plan file: `/home/theyusuf/.claude/plans/first-plan-zippy-clover.md` for current task scratchpad.
