import { Badge } from "@/components/ui/badge"
import type { AlertDelivery } from "@/features/alerts/types"

/**
 * An alert can sit in "firing" while its email died against a dead SMTP host.
 * Delivery state is therefore shown per row, not inferred from alert state.
 */
export function DeliveryBadge({ delivery }: { delivery?: AlertDelivery }) {
  if (!delivery) return <span className="text-muted-foreground text-xs">—</span>

  if (delivery.status === "failed") {
    return (
      <Badge variant="destructive" title={delivery.last_error ?? "delivery failed"}>
        Failed
      </Badge>
    )
  }
  if (delivery.status === "sent") return <Badge variant="outline">Sent</Badge>
  return (
    <Badge variant="secondary" title={delivery.last_error ?? undefined}>
      Retrying ({delivery.attempts})
    </Badge>
  )
}
