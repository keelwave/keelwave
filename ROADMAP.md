# keelwave roadmap

Master plan across `core/`, `python/`, `ts/`. Per-repo sections are the
authoritative task list for that repo.

Status date: 2026-08-18. Current: core untagged, `keelwave` 0.1.0 on PyPI,
`keelwave` 0.1.0 on npm.

---

## 1. The problem this roadmap solves

Every defect found so far is the same shape: **keelwave reports success on a
run that failed.** A tool returns nothing and we record `tool_success = true`.
An agent reasons for 50 steps and we record zero. A structured-output agent
runs ten tools and we record none.

An observability product that under-reports failure is worse than no product,
because it converts "I don't know" into a false "it's fine". Everything in
Phase 1 exists to close that gap. Nothing ships as 1.0 until it is closed.

---

## 2. Confirmed defects

All verified by running code, not by reading docs. Each line names the file.

### Trace correctness — highest severity

| # | repo | file | defect |
|---|---|---|---|
| 1 | python | `adapters/pydantic_ai.py` | agents using `output_type=` record **zero** steps — output tools arrive as `OutputToolCallEvent`, which we never check |
| 2 | python | `adapters/pydantic_ai.py` | provider-native tools (web search, code exec) record zero steps — v2 removed `BuiltinToolCallEvent`; they surface only via `PartStartEvent`/`PartDeltaEvent` |
| 3 | python | `adapters/pydantic_ai.py` | `ModelRetry` (tool failure) recorded as `ok=True`; the `getattr(part, "is_error", False)` guard checks an attribute no part type has |
| 4 | python | `adapters/pydantic_ai.py` | falsy tool output (`0`, `""`, `[]`, `False`) stored as `{}` — `if output_str else {}` |
| 5 | all | SDK-wide | `tool_success` defaults to `true` and no adapter ever passes `false`; dashboard tool success rate is currently not measuring anything |

### Reasoning capture

| # | repo | file | defect |
|---|---|---|---|
| 6 | python | `adapters/pydantic_ai.py` | `ThinkingPart` unhandled; module docstring claims "emits think/tool_call/tool_result steps" |
| 7 | python | `adapters/anthropic.py` | `redacted_thinking` blocks dropped — filtered by `type == "thinking"` and read via `.thinking`, but the block is typed `redacted_thinking` and carries `.data`. Reads as "model didn't reason" |
| 8 | python | `adapters/anthropic.py` | whole-call tokens attributed to **each** thinking block; two blocks double-count. `usage.output_tokens_details.thinking_tokens` exists on `anthropic>=0.109` and is the correct source |
| 9 | python | `adapters/openai.py` | Responses API not wrapped at all — only `chat.completions.create`. OpenAI's reasoning models are Responses-first |
| 10 | python | `adapters/openai.py` | `output_tokens_details.reasoning_tokens` ignored. This is the one provider that hands over the reasoning/answer token split for free |
| 11 | ts | `adapters/vercel-ai.ts` | `wrapStream` captures no reasoning — the transform only inspects `finish` and `error` chunks |
| 12 | python | `adapters/anthropic.py`, `openai.py` | no streaming reasoning capture (`thinking_delta`, reasoning deltas) |

Net effect of 6–12: **reasoning is captured only on the non-streaming path,
and only for Anthropic plain thinking blocks and DeepSeek-via-OpenAI-format.**
Every streaming user gets zero, silently.

### Packaging

| # | repo | defect |
|---|---|---|
| 13 | python | `pydantic-ai>=1.107.0` is a hard dependency — installed for everyone, including users who only touch the OpenAI adapter |
| 14 | python | `openai` and `anthropic` are imported by adapters but declared nowhere |
| 15 | python | `pydantic-ai>=1.107.0` has no upper bound; v2.31 already installs untested (it works — verified — but by luck, not by test) |

### Server

| # | repo | defect |
|---|---|---|
| 16 | core | `internal/store/agent_runs.go` `ListByProject` reads the stale `total_steps` column, so a running agent's step count does not advance live |
| 17 | core | step-unit mismatch: SDK `total_steps` counts every emitted step; `maxSteps` in agent frameworks counts loop iterations. A run with `maxSteps: 50` legitimately reports 135 steps. Needs either a second column or documented semantics |

---

## 3. Version support policy

The recurring question is "which version do we support". The answer that
scales is: **declare a wide range, detect shape at runtime, test the edges.**

Adapters read a handful of fields from a fast-moving upstream. Pinning
narrowly means a release treadmill; pinning wide without detection means
silent zero-capture. So:

1. **Declare the range** in package metadata, with an upper bound. An
   unbounded range is how untested majors reach users.
2. **Detect, don't assume.** Read a field by trying each known shape in turn
   and taking the first that matches. Already the pattern in
   `ts/src/adapters/vercel-ai.ts:readReasoning`; extend it rather than
   branching on a version number.
3. **Warn once when nothing matches.** A shape we do not recognise must
   produce one `warnings.warn` / `console.warn`, not silence. This is the
   single most important rule on this page — it converts every future
   upstream rename from a silent data hole into a visible message.
4. **CI matrix pins the edges only** — oldest supported and newest supported.
   The middle is covered by the shape detection.

### Concrete ranges

| SDK | dependency | today | target |
|---|---|---|---|
| ts | `ai` | peer `^4.3.19 \|\| ^5.0.0` | `^4 \|\| ^5 \|\| ^6 \|\| ^7`, matrix on 4.3.19 and latest 7.x |
| ts | `openai` | — | peer `^4 \|\| ^5`, optional |
| ts | `@anthropic-ai/sdk` | — | peer `^0.3x \|\| ^1`, optional |
| python | `pydantic-ai` | `>=1.107.0` required | extra `[pydantic-ai]`, `>=1.107,<3`, matrix on 1.107 and 2.31 |
| python | `openai` | undeclared | extra `[openai]`, `>=1.60,<3` |
| python | `anthropic` | undeclared | extra `[anthropic]`, `>=0.109,<1` |

`pip install keelwave` installs the client and nothing else.
`pip install keelwave[openai]` adds one adapter's dependency.

### Note on pydantic-ai v1 → v2

Verified by running the adapter against both 1.107.0 and 2.31.0: **identical
output, no code change needed.** Everything the migration guide renames, our
floor of 1.107 already sits on the new side of — the guide states most V2
removals were deprecated as of v1.100.0. The guide also states explicitly that
the `event_stream_handler=` argument on `run()` is unchanged; only the `Agent()`
constructor kwarg moved to capabilities.

One behaviour change does affect traces without renaming any symbol: the
default `end_strategy` moved from `'early'` to `'graceful'`, so function tools
requested alongside a successful output tool now execute instead of being
skipped. Same agent, same code, **more tool_call steps on v2**. Step counts are
not comparable across a user's upgrade. Document this.

---

## 4. Adapter coverage target

| adapter | python | ts |
|---|---|---|
| OpenAI — chat completions | partial (no reasoning tokens) | **missing** |
| OpenAI — responses | **missing** | **missing** |
| Anthropic | partial (no redacted, no stream) | **missing** |
| Vercel AI SDK | n/a | partial (v4/v5, no stream reasoning) |
| pydantic-ai | partial | n/a |

ts has one adapter against python's three. Closing that is Phase 2.

---

## 5. Phases

### Phase 1 — trace correctness (blocks everything)

Defects 1–5, then 6–12. Ship as `0.2.0` on both SDKs.

- [ ] `tool_success` means something: every adapter passes explicit `ok`
- [ ] pydantic-ai: handle `OutputToolCallEvent` / `OutputToolResultEvent`
- [ ] pydantic-ai: handle `PartStartEvent` / `PartDeltaEvent` for native tools
- [ ] pydantic-ai: `RetryPromptPart` and v2 `ToolFailed` → `ok=False`
- [ ] pydantic-ai: falsy outputs round-trip
- [ ] pydantic-ai: emit `ThinkingPart` as a think step
- [ ] anthropic: `redacted_thinking` → think step flagged encrypted, never dropped
- [ ] anthropic: read `output_tokens_details.thinking_tokens`
- [ ] openai: read `output_tokens_details.reasoning_tokens`
- [ ] openai: wrap the Responses API
- [ ] ts: capture reasoning on `wrapStream`
- [ ] ts: support `ai` v6 and v7 reasoning shapes
- [ ] all: warn once on unrecognised response shape

### Phase 2 — adapter parity + packaging

Ship as `0.3.0`.

- [ ] ts: OpenAI adapter (chat completions + responses)
- [ ] ts: Anthropic adapter
- [ ] python: move provider deps to extras
- [ ] both: CI matrix across the declared version ranges
- [ ] docs: a supported-versions table, kept honest by the matrix

### Phase 3 — server + schema

- [ ] live step count for running agents (defect 16)
- [ ] step-unit semantics (defect 17)
- [ ] reasoning fields on the wire: `is_encrypted`, `is_summary`,
      `token_count_source`, `reasoning_source` (native vs extracted).
      Server first — SDKs cannot send what the schema will not accept
- [ ] access controls on reasoning text: raw chain-of-thought is less filtered
      than the final answer and must not inherit the answer's permissions

### Phase 4 — 1.0

**Not yet.** 1.0 is a compatibility promise on the wire protocol and both SDK
surfaces. Three things must be true first:

1. Trace correctness closed — a green dashboard means the agent actually
   worked. Today it does not.
2. Wire protocol frozen — Phase 3 adds reasoning columns. Adding fields after
   1.0 is fine; renaming them is not.
3. Real usage — at least one agent traced end to end by someone who did not
   write the SDK.

Sequence: `0.2.0` correctness → `0.3.0` parity → `0.4.0` server → tag core
`v0.4.0` (core has never been tagged) → soak → `1.0.0` across all three repos
together, sharing a version number from that point on.

Releasing 1.0 before item 1 ships a promise we would immediately break.
