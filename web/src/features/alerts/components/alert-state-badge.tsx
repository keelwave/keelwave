import { Badge } from "@/components/ui/badge"
import type { Alert } from "@/features/alerts/types"

/**
 * A resolved alert that never carried fired_at cleared during for_seconds —
 * the engine dropped it before notifying. Showing that separately tells the
 * user their damping worked instead of hiding a near miss.
 */
export function alertDisplayState(alert: Alert): string {
  if (alert.state === "resolved" && !alert.fired_at) return "Cleared before firing"
  switch (alert.state) {
    case "pending":
      return "Pending"
    case "firing":
      return "Firing"
    case "recovering":
      return "Recovering"
    case "resolved":
      return "Resolved"
  }
}

export function AlertStateBadge({ alert }: { alert: Alert }) {
  const label = alertDisplayState(alert)
  const variant =
    alert.state === "firing"
      ? "destructive"
      : alert.state === "pending" || alert.state === "recovering"
        ? "secondary"
        : "outline"
  return <Badge variant={variant}>{label}</Badge>
}
