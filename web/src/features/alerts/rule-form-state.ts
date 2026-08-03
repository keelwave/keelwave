import { isAggregateSignal } from "./types"
import type {
  AlertRule,
  AlertRuleInput,
  AlertRulePreviewInput,
  AlertSignal,
} from "./types"

export interface RuleDraft {
  name: string
  signal: AlertSignal
  evaluateOverWindow: boolean
  agentName: string
  comparator: string
  threshold: number
  windowSeconds: number
  severity: "page" | "warn" | "digest"
  recipient: string
  enabled: boolean
  forSeconds: number
  keepFiringForSeconds: number
  cooldownSeconds: number
  minRequests: number
}

export function emptyDraft(): RuleDraft {
  return {
    name: "",
    signal: "loop",
    evaluateOverWindow: false,
    agentName: "",
    comparator: ">",
    threshold: 0,
    windowSeconds: 300,
    severity: "warn",
    recipient: "",
    enabled: true,
    forSeconds: 0,
    keepFiringForSeconds: 0,
    cooldownSeconds: 0,
    minRequests: 0,
  }
}

export function draftFromRule(rule: AlertRule): RuleDraft {
  return {
    name: rule.name,
    signal: rule.signal,
    evaluateOverWindow: rule.class === "aggregate",
    agentName: rule.agent_name ?? "",
    comparator: rule.comparator ?? ">",
    threshold: rule.threshold,
    windowSeconds: rule.window_seconds ?? 300,
    severity: rule.severity,
    recipient: rule.channel_config.to ?? "",
    enabled: rule.enabled,
    forSeconds: rule.for_seconds,
    keepFiringForSeconds: rule.keep_firing_for_seconds,
    cooldownSeconds: rule.cooldown_seconds,
    minRequests: rule.min_requests,
  }
}

/**
 * loop only ever fires on run-finish. run_failure can do both: on its own it is
 * an event rule, or it can be evaluated as a completion rate over a window.
 * Everything else is a scheduled metric.
 */
export function classForSignal(
  signal: AlertSignal,
  evaluateOverWindow: boolean
): "event" | "aggregate" {
  if (signal === "loop") return "event"
  if (signal === "run_failure") return evaluateOverWindow ? "aggregate" : "event"
  return "aggregate"
}

export function draftClass(draft: RuleDraft): "event" | "aggregate" {
  return classForSignal(draft.signal, draft.evaluateOverWindow)
}

export function toRuleInput(draft: RuleDraft): AlertRuleInput {
  const cls = draftClass(draft)
  const input: AlertRuleInput = {
    name: draft.name,
    class: cls,
    signal: draft.signal,
    threshold: cls === "aggregate" ? draft.threshold : 0,
    severity: draft.severity,
    channel: "email",
    channel_config: { to: draft.recipient },
    enabled: draft.enabled,
    for_seconds: draft.forSeconds,
    keep_firing_for_seconds: draft.keepFiringForSeconds,
    cooldown_seconds: draft.cooldownSeconds,
    min_requests: draft.minRequests,
  }
  if (draft.agentName.trim()) input.agent_name = draft.agentName.trim()
  if (cls === "aggregate") {
    input.comparator = draft.comparator
    input.window_seconds = draft.windowSeconds
  }
  return input
}

/** Null when the draft has no metric to evaluate — event rules have no threshold. */
export function toPreviewInput(draft: RuleDraft): AlertRulePreviewInput | null {
  if (draftClass(draft) !== "aggregate") return null
  if (!isAggregateSignal(draft.signal)) return null
  const input: AlertRulePreviewInput = {
    signal: draft.signal,
    comparator: draft.comparator,
    threshold: draft.threshold,
    window_seconds: draft.windowSeconds,
    min_requests: draft.minRequests,
  }
  if (draft.agentName.trim()) input.agent_name = draft.agentName.trim()
  return input
}
