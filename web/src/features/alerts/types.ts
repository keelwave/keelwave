/**
 * Mirrors the alert_events.state CHECK constraint. "fired" is the terminal state
 * event-class alerts (loop, run_failure) land in — they have no window to
 * recover over, so they never pass through pending/recovering.
 */
export type AlertState = "pending" | "firing" | "recovering" | "resolved" | "fired"

export type AlertSignal =
  | "run_failure"
  | "loop"
  | "termination_shift"
  | "cost_burn"
  | "tool_failure"
  | "duration_p95"
  | "eval_regression"

export type AlertDeliveryStatus = "pending" | "processing" | "done" | "dead"

export interface AlertDelivery {
  status: AlertDeliveryStatus
  attempts: number
  last_error?: string
}

export interface Alert {
  id: string
  rule_id: string
  project_id: string
  scope_label: string
  state: AlertState
  first_breached_at?: string
  fired_at?: string
  resolved_at?: string
  recovering_since?: string
  last_fired_at?: string
  last_value?: number
  last_evaluated_at: string
  delivery?: AlertDelivery
}

export interface AlertRule {
  id: string
  project_id: string
  agent_name?: string
  name: string
  class: "event" | "aggregate"
  signal: AlertSignal
  comparator?: string
  threshold: number
  window_seconds?: number
  severity: "page" | "warn" | "digest"
  for_seconds: number
  keep_firing_for_seconds: number
  cooldown_seconds: number
  min_requests: number
  channel: string
  channel_config: { to?: string }
  enabled: boolean
  created_at: string
}

export interface AlertRuleInput {
  name: string
  class: "event" | "aggregate"
  signal: AlertSignal
  agent_name?: string
  comparator?: string
  threshold: number
  window_seconds?: number
  severity: "page" | "warn" | "digest"
  for_seconds?: number
  keep_firing_for_seconds?: number
  cooldown_seconds?: number
  min_requests?: number
  channel: "email"
  channel_config: { to: string }
  enabled: boolean
}

export interface AlertRulePreviewInput {
  signal: AlertSignal
  agent_name?: string
  comparator: string
  threshold: number
  window_seconds?: number
  min_requests?: number
}

export interface AlertRulePreviewResult {
  value: number
  sample_count: number
  would_breach: boolean
  scope_label: string
}

/**
 * Signals the engine evaluates on a schedule vs. on run-finish. Drives which
 * fields the rule form shows; the server's validator stays authoritative.
 */
export const AGGREGATE_SIGNALS: AlertSignal[] = [
  "run_failure",
  "termination_shift",
  "cost_burn",
  "tool_failure",
  "duration_p95",
  "eval_regression",
]

export const EVENT_SIGNALS: AlertSignal[] = ["loop", "run_failure"]

export function isAggregateSignal(signal: AlertSignal): boolean {
  return AGGREGATE_SIGNALS.includes(signal)
}
