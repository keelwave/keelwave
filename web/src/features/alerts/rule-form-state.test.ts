import { describe, expect, it } from "vitest"

import {
  classForSignal,
  comparatorOptions,
  draftFromRule,
  emptyDraft,
  lowerIsBad,
  naturalComparator,
  toPreviewInput,
  toRuleInput,
} from "./rule-form-state"
import type { AlertRule } from "./types"

describe("classForSignal", () => {
  it("keeps loop event-class regardless of the window switch", () => {
    expect(classForSignal("loop", false)).toBe("event")
    expect(classForSignal("loop", true)).toBe("event")
  })

  it("treats run_failure as event unless evaluated over a window", () => {
    expect(classForSignal("run_failure", false)).toBe("event")
    expect(classForSignal("run_failure", true)).toBe("aggregate")
  })

  it("treats metric signals as aggregate", () => {
    expect(classForSignal("cost_burn", false)).toBe("aggregate")
    expect(classForSignal("duration_p95", false)).toBe("aggregate")
  })
})

describe("toPreviewInput", () => {
  it("returns null for event-class drafts (nothing to evaluate)", () => {
    const draft = { ...emptyDraft(), signal: "loop" as const }
    expect(toPreviewInput(draft)).toBeNull()
  })

  it("builds a preview payload for aggregate drafts", () => {
    const draft = {
      ...emptyDraft(),
      signal: "cost_burn" as const,
      comparator: ">",
      threshold: 5,
      windowSeconds: 300,
      agentName: "researcher",
    }
    expect(toPreviewInput(draft)).toEqual({
      signal: "cost_burn",
      comparator: ">",
      threshold: 5,
      window_seconds: 300,
      agent_name: "researcher",
      min_requests: 0,
    })
  })
})

describe("toRuleInput", () => {
  it("omits comparator and window for event-class rules", () => {
    const draft = {
      ...emptyDraft(),
      name: "loops",
      signal: "loop" as const,
      recipient: "ops@acme.com",
    }
    const input = toRuleInput(draft)
    expect(input.class).toBe("event")
    expect(input.comparator).toBeUndefined()
    expect(input.window_seconds).toBeUndefined()
    expect(input.channel_config).toEqual({ to: "ops@acme.com" })
  })

  it("drops an empty agent name so the rule stays project-wide", () => {
    const draft = { ...emptyDraft(), name: "cost", signal: "cost_burn" as const, agentName: "" }
    expect(toRuleInput(draft).agent_name).toBeUndefined()
  })
})

// Server contract (cmd/api/alerts.go): lowerIsBad is true only for run_failure
// and eval_regression, which may only use < or <=. Every other aggregate signal
// is higher-is-bad and may only use > or >=. An empty comparator is allowed
// server-side (it fills the natural default), but this form always sends one.
describe("lowerIsBad / naturalComparator / comparatorOptions", () => {
  it("flags run_failure and eval_regression as lower-is-bad", () => {
    expect(lowerIsBad("run_failure")).toBe(true)
    expect(lowerIsBad("eval_regression")).toBe(true)
  })

  it("flags every other aggregate signal as higher-is-bad", () => {
    expect(lowerIsBad("cost_burn")).toBe(false)
    expect(lowerIsBad("termination_shift")).toBe(false)
    expect(lowerIsBad("tool_failure")).toBe(false)
    expect(lowerIsBad("duration_p95")).toBe(false)
    expect(lowerIsBad("loop")).toBe(false)
  })

  it("picks < for lower-is-bad signals and > for the rest", () => {
    expect(naturalComparator("run_failure")).toBe("<")
    expect(naturalComparator("eval_regression")).toBe("<")
    expect(naturalComparator("cost_burn")).toBe(">")
    expect(naturalComparator("duration_p95")).toBe(">")
  })

  it("offers only the valid comparator direction per signal", () => {
    expect(comparatorOptions("run_failure")).toEqual(["<", "<="])
    expect(comparatorOptions("eval_regression")).toEqual(["<", "<="])
    expect(comparatorOptions("cost_burn")).toEqual([">", ">="])
    expect(comparatorOptions("duration_p95")).toEqual([">", ">="])
  })
})

describe("comparator direction upheld by payload builders", () => {
  it("toRuleInput sends a <-direction comparator for run_failure windowed", () => {
    const draft = {
      ...emptyDraft(),
      name: "windowed failures",
      signal: "run_failure" as const,
      evaluateOverWindow: true,
      comparator: naturalComparator("run_failure"),
      recipient: "ops@acme.com",
    }
    expect(toRuleInput(draft).comparator).toMatch(/^<=?$/)
  })

  it("toRuleInput sends a <-direction comparator for eval_regression", () => {
    const draft = {
      ...emptyDraft(),
      name: "eval regression",
      signal: "eval_regression" as const,
      comparator: naturalComparator("eval_regression"),
      recipient: "ops@acme.com",
    }
    expect(toRuleInput(draft).comparator).toMatch(/^<=?$/)
  })

  it("toRuleInput sends a >-direction comparator for every other aggregate signal", () => {
    for (const signal of [
      "cost_burn",
      "termination_shift",
      "tool_failure",
      "duration_p95",
    ] as const) {
      const draft = {
        ...emptyDraft(),
        name: "metric",
        signal,
        comparator: naturalComparator(signal),
        recipient: "ops@acme.com",
      }
      expect(toRuleInput(draft).comparator).toMatch(/^>=?$/)
    }
  })

  it("toPreviewInput mirrors the same direction constraint", () => {
    const draft = {
      ...emptyDraft(),
      signal: "eval_regression" as const,
      comparator: naturalComparator("eval_regression"),
      threshold: 0.5,
    }
    expect(toPreviewInput(draft)?.comparator).toMatch(/^<=?$/)
  })
})

describe("window_seconds is never non-positive on the wire", () => {
  it("toRuleInput omits window_seconds when the draft window is 0", () => {
    const draft = {
      ...emptyDraft(),
      name: "cost",
      signal: "cost_burn" as const,
      windowSeconds: 0,
      recipient: "ops@acme.com",
    }
    const input = toRuleInput(draft)
    if (input.window_seconds !== undefined) {
      expect(input.window_seconds).toBeGreaterThan(0)
    }
  })

  it("toRuleInput omits window_seconds when the draft window is negative", () => {
    const draft = {
      ...emptyDraft(),
      name: "cost",
      signal: "cost_burn" as const,
      windowSeconds: -30,
      recipient: "ops@acme.com",
    }
    const input = toRuleInput(draft)
    if (input.window_seconds !== undefined) {
      expect(input.window_seconds).toBeGreaterThan(0)
    }
  })

  it("toPreviewInput never sends a non-positive window_seconds", () => {
    const draft = {
      ...emptyDraft(),
      signal: "cost_burn" as const,
      windowSeconds: 0,
    }
    const input = toPreviewInput(draft)
    if (input && input.window_seconds !== undefined) {
      expect(input.window_seconds).toBeGreaterThan(0)
    }
  })
})

describe("a non-finite threshold or window never serializes to null", () => {
  it("toRuleInput guards a NaN threshold", () => {
    const draft = {
      ...emptyDraft(),
      name: "cost",
      signal: "cost_burn" as const,
      threshold: Number.NaN,
      recipient: "ops@acme.com",
    }
    const wire = JSON.parse(JSON.stringify(toRuleInput(draft))) as Record<string, unknown>
    expect(wire.threshold).not.toBeNull()
    expect(Number.isFinite(wire.threshold)).toBe(true)
  })

  it("toRuleInput guards a NaN window_seconds", () => {
    const draft = {
      ...emptyDraft(),
      name: "cost",
      signal: "cost_burn" as const,
      windowSeconds: Number.NaN,
      recipient: "ops@acme.com",
    }
    const wire = JSON.parse(JSON.stringify(toRuleInput(draft))) as Record<string, unknown>
    expect(wire.window_seconds).not.toBeNull()
  })

  it("toPreviewInput refuses to build a payload for a non-finite draft", () => {
    const draft = {
      ...emptyDraft(),
      signal: "cost_burn" as const,
      threshold: Number.NaN,
    }
    expect(toPreviewInput(draft)).toBeNull()
  })

  it("toPreviewInput refuses to build a payload for a non-finite window", () => {
    const draft = {
      ...emptyDraft(),
      signal: "cost_burn" as const,
      windowSeconds: Number.NaN,
    }
    expect(toPreviewInput(draft)).toBeNull()
  })
})

describe("draftFromRule preserves an existing rule's comparator", () => {
  it("does not overwrite a stored comparator that disagrees with the signal's natural direction key", () => {
    const rule: AlertRule = {
      id: "r1",
      project_id: "p1",
      name: "eval regression",
      class: "aggregate",
      signal: "eval_regression",
      comparator: "<=",
      threshold: 0.7,
      window_seconds: 600,
      severity: "warn",
      for_seconds: 0,
      keep_firing_for_seconds: 0,
      cooldown_seconds: 900,
      min_requests: 0,
      channel: "email",
      channel_config: { to: "ops@acme.com" },
      enabled: true,
      created_at: "2026-01-01T00:00:00Z",
    }
    expect(draftFromRule(rule).comparator).toBe("<=")
  })
})
