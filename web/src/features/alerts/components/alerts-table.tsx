import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { AlertStateBadge } from "@/features/alerts/components/alert-state-badge"
import { DeliveryBadge } from "@/features/alerts/components/delivery-badge"
import type { Alert } from "@/features/alerts/types"

function formatTime(value?: string) {
  if (!value) return "—"
  return new Date(value).toLocaleString()
}

export function AlertsTable({
  alerts,
  ruleNames,
}: {
  alerts: Alert[]
  ruleNames: Map<string, string>
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Rule</TableHead>
          <TableHead>Scope</TableHead>
          <TableHead>State</TableHead>
          <TableHead>Value</TableHead>
          <TableHead>Fired</TableHead>
          <TableHead>Delivery</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {alerts.map((alert) => (
          <TableRow key={alert.id}>
            <TableCell className="font-medium">
              {ruleNames.get(alert.rule_id) ?? "Deleted rule"}
            </TableCell>
            <TableCell>{alert.scope_label || "All agents"}</TableCell>
            <TableCell>
              <AlertStateBadge alert={alert} />
            </TableCell>
            <TableCell>
              {alert.last_value != null ? alert.last_value.toFixed(2) : "—"}
            </TableCell>
            <TableCell>{formatTime(alert.fired_at)}</TableCell>
            <TableCell>
              <DeliveryBadge delivery={alert.delivery} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
