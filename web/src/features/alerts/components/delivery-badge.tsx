import { Badge } from "@/components/ui/badge"
import type { AlertDelivery } from "@/features/alerts/types"

/**
 * An alert can sit in "firing" while its email died against a dead SMTP host.
 * Delivery state is therefore shown per row, not inferred from alert state.
 *
 * notification_jobs.status is constrained to pending|processing|done|dead
 * (cmd/migrate/migrations/000011_notification_jobs_processing_status.up.sql).
 * "sent"/"failed" never appear on the wire — done/dead map to them here.
 */
export function DeliveryBadge({ delivery }: { delivery?: AlertDelivery }) {
  if (!delivery) return <span className="text-muted-foreground text-xs">—</span>

  switch (delivery.status) {
    case "dead":
      return (
        <Badge variant="destructive" title={delivery.last_error ?? "delivery failed"}>
          Failed
        </Badge>
      )
    case "done":
      return <Badge variant="outline">Sent</Badge>
    case "pending":
    case "processing":
      return (
        <Badge variant="secondary" title={delivery.last_error ?? undefined}>
          Retrying ({delivery.attempts})
        </Badge>
      )
    default:
      return <Badge variant="secondary">{delivery.status}</Badge>
  }
}
