import { describe, expect, it } from "vitest"

import {
  classForSignal,
  emptyDraft,
  toPreviewInput,
  toRuleInput,
} from "./rule-form-state"

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
