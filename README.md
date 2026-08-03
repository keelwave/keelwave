# keelwave

**Open-source, self-hostable observability and alerting for AI agents.**

keelwave captures what your agents actually do — the decision loop, the tool calls, the tokens burned, the reason a run ended — and alerts you when they go wrong. It's built for the failure mode generic monitoring can't see: the agent that keeps running while your infra dashboard stays green. One Go binary, one Postgres/TimescaleDB database, a React dashboard. `docker compose up` and go.

> **Status:** early / MVP, self-host first. The agent ingest, query API, and rule-driven email alerting all work and are tested. There is no hosted cloud offering yet. Expect rough edges, and read the roadmap before betting production on it.

---

## The problem: agents fail silently

Infrastructure monitoring answers "is the service up?" — latency, error rate, throughput. For AI agents, all of those can look perfect while the agent is quietly failing:

- It gets **stuck in a loop**, calling the same tool with the same input over and over.
- It **burns tokens** wandering toward an answer it never reaches.
- It hits a wall and terminates for the wrong reason — context limit, max steps — instead of finishing cleanly.
- It produces **wrong output** that no HTTP status code will ever flag.

Latency is fine. Error rate is 0%. The run was still a failure. Tools that measure infrastructure or dump raw traces don't tell you *why* an agent went wrong or *where* in its reasoning it broke.

## What keelwave captures

keelwave records the agent's actual behavior, not just the requests around it:

- **Decision traces** — every step in the loop: `think → tool_call → tool_result → replan`, in order, per run.
- **Loop detection** — a SHA-256 fingerprint of `tool_name + input` on each step; a repeated fingerprint in the same run is a loop, flagged automatically.
- **Tool analytics** — success rate, p95 latency, and failure reasons per tool, from an auto-populated tool registry.
- **Token efficiency** — tokens per step, so you can see reasoning degrade before the run fails.
- **Eval scores** — per-run quality scores (correctness, completeness, efficiency, safety) attached to a run.
- **Explicit termination reason** — `clean`, `max_steps_reached`, `context_limit`, `error`, `loop_detected`, `timeout` — not a guess.
- **LLM call traces** — model, tokens, cost, latency, and status for every underlying model call, linked back to its run.
- **Rule-driven alerting** — fire an alert when an agent signal crosses a line (cost, failure rate, tool failure, duration, eval regression, loop), delivered by email. See [Alerting](#alerting).

## Quickstart

### 1. Run the stack

Start the database with Docker Compose, apply migrations, and seed a dev project + API key:

```bash
# from core/
docker compose up -d db          # TimescaleDB (pg18)
make migrate-up                  # apply all migrations
make seed                        # creates a dev project + prints a plaintext kw_... API key
make run                         # start the Go API on :8080  (or `make dev` for hot reload)
```

Copy the `kw_...` key that `make seed` prints — that's what the SDK authenticates with. Configuration is read from the environment (`DB_ADDR`, `ADDR`, and mailer settings for alerts); see `.envrc` for the full set.

> Some runtime names are mid-rename from an earlier `vigil` branding. The default local DB user/password/name are all `keelwave` (see `docker-compose.yml`), and the API key prefix is `kw_`.

### 2. Trace an agent (Python)

```bash
pip install keelwave
```

```python
import os
from keelwave import Keelwave

client = Keelwave(
    api_key=os.environ["KEELWAVE_API_KEY"],          # your kw_... key
    endpoint=os.environ.get("KEELWAVE_ENDPOINT", "http://localhost:8080"),
)

# Any function decorated with @observe becomes a step in the run.
# Duplicate tool call fingerprints are detected as loops automatically.
@client.observe(name="web_search", step_type="tool_call")
def web_search(q: str) -> dict:
    return {"results": search(q)}

# @agent opens and closes the run for you; steps inside are linked to it.
@client.agent(name="research-agent")
def run_agent(question: str) -> str:
    results = web_search(q=question)
    return summarize(results)

run_agent("what changed in our churn numbers last week?")
```

The SDK also wraps provider clients to capture model calls automatically:

```python
import anthropic
claude = client.wrap_anthropic(anthropic.Anthropic())
# every claude.messages.create(...) now emits an LLM trace linked to the active run
```

Open the dashboard to explore runs, step timelines, tool stats, and flagged loops.

## Architecture

keelwave is deliberately small: **one binary, one database.** A single Go service (`chi`, `pgx/v5`) handles both ingest (`/v1/ingest/*`, `/v1/agent/*`) and the query API the dashboard reads. Storage is **TimescaleDB** (Postgres + the TimescaleDB extension) — agent runs and steps are hypertables, and alerting reads a continuous aggregate (`agent_runs_5m`) for windowed metrics. Hot-path inserts are buffered and written with `COPY`; loop fingerprints are computed server-side on ingest. The dashboard is a **React 19 + TypeScript** app (Recharts, shadcn/ui). SDKs push data over HTTP — Python today, TypeScript and Go planned. No Redis, no ClickHouse, no `pg_cron`, no extra extensions.

## Alerting

The alerting engine turns noisy agent signals into a small number of trustworthy emails. You define **rules** over agent metrics — cost burn, run failure rate, termination shift, p95 duration, tool failure, eval regression, or loop events — and keelwave evaluates them on a ticker (aggregates) or on run-finish (events).

It's built to not spam you. A state machine with hysteresis (`for` / `keep_firing_for`) fires **once when a condition starts and once when it's over**, never in between, and a no-data window never fires. Event alerts (a run looped, a run failed) fire once per cooldown window instead of once per run. Detection and delivery are decoupled through a transactional outbox, so a flaky mail provider never blocks evaluation or silently drops an alert.

Email (via Resend) is the delivery channel today; Slack, webhook, and PagerDuty are the planned next channels. For a full walkthrough of the design, see [`docs/notes/alerting-engine.md`](docs/notes/alerting-engine.md).

## Status & roadmap

keelwave is **pre-launch and self-host first.** What works today:

- Agent + LLM ingest, server-side loop fingerprinting, batched writes.
- Agent inspector query API (runs, steps, loops, tool stats).
- Rule-driven alerting with email delivery.
- Python SDK (`keelwave`) with `@agent` / `@observe` decorators and provider wrappers.
- TypeScript SDK (`keelwave`) with `agent` / `observe` wrappers and a Vercel AI SDK adapter.
- React dashboard.

Planned / in progress:

- More alert channels (Slack, webhook, PagerDuty) and an in-dashboard alert UI.
- Go SDK.
- OpenTelemetry / OTLP ingest.
- A hosted cloud tier.

**Scope note:** keelwave is intentionally focused on **AI agents**, not general systems monitoring. There is dormant, tested ingest for HTTP events and host metrics in the codebase, but it is not part of the product surface today — see [`docs/notes/scope.md`](docs/notes/scope.md).

## Related repositories

| Repo | What |
|---|---|
| [`keelwave`](https://github.com/keelwave/keelwave) | Core — Go API, TimescaleDB schema, React dashboard (this repo) |
| [`keelwave-python`](https://github.com/keelwave/keelwave-python) | Python SDK |
| [`keelwave-ts`](https://github.com/keelwave/keelwave-ts) | TypeScript SDK |

## License

Apache License 2.0. See [LICENSE](LICENSE).
