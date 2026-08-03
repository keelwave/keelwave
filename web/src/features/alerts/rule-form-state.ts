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

/**
 * Mirrors cmd/api/alerts.go lowerIsBad: run_failure and eval_regression are
 * "bad when the metric falls" (completion rate, correctness), so their only
 * valid comparator direction is < / <=. Every other aggregate signal is
 * higher-is-bad (cost, failure share, latency) and only accepts > / >=.
 * validAggregateComparator on the server rejects the wrong direction with a
 * 400, so the form must never construct one.
 */
export function lowerIsBad(signal: AlertSignal): boolean {
  return signal === "run_failure" || signal === "eval_regression"
}

/** The signal's natural comparator direction — mirrors naturalComparator in alerts.go. */
export function naturalComparator(signal: AlertSignal): string {
  return lowerIsBad(signal) ? "<" : ">"
}

/** The only comparator values validAggregateComparator accepts for this signal. */
export function comparatorOptions(signal: AlertSignal): string[] {
  return lowerIsBad(signal) ? ["<", "<="] : [">", ">="]
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

/**
 * The server tags window_seconds `omitempty,gt=0` on both the rule and preview
 * payloads: a non-positive or non-finite (NaN serializes to null, which still
 * passes omitempty) window must never be sent. Treat it as "use the server's
 * default" by omitting the field entirely rather than clamping to 1 — clamping
 * would silently save a materially different window than what the user saw.
 */
function positiveWindowOrUndefined(windowSeconds: number): number | undefined {
  return Number.isFinite(windowSeconds) && windowSeconds > 0 ? windowSeconds : undefined
}

export function toRuleInput(draft: RuleDraft): AlertRuleInput {
  const cls = draftClass(draft)
  const threshold = cls === "aggregate" && Number.isFinite(draft.threshold) ? draft.threshold : 0
  const input: AlertRuleInput = {
    name: draft.name,
    class: cls,
    signal: draft.signal,
    threshold,
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
    const window = positiveWindowOrUndefined(draft.windowSeconds)
    if (window !== undefined) input.window_seconds = window
  }
  return input
}

/**
 * Null when the draft has no metric to evaluate — event rules have no
 * threshold — or when threshold/window are non-finite: a NaN threshold or
 * window is a mid-edit draft (e.g. an emptied number input), not something
 * worth round-tripping to the preview endpoint.
 */
export function toPreviewInput(draft: RuleDraft): AlertRulePreviewInput | null {
  if (draftClass(draft) !== "aggregate") return null
  if (!isAggregateSignal(draft.signal)) return null
  if (!Number.isFinite(draft.threshold)) return null
  if (!Number.isFinite(draft.windowSeconds)) return null
  const input: AlertRulePreviewInput = {
    signal: draft.signal,
    comparator: draft.comparator,
    threshold: draft.threshold,
    min_requests: draft.minRequests,
  }
  const window = positiveWindowOrUndefined(draft.windowSeconds)
  if (window !== undefined) input.window_seconds = window
  if (draft.agentName.trim()) input.agent_name = draft.agentName.trim()
  return input
}
