import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  useDeleteAlertRule,
  useUpdateAlertRule,
} from "@/features/alerts/hooks/use-alerts"
import type { AlertRule } from "@/features/alerts/types"

export function RulesTable({
  orgId,
  projectId,
  rules,
  canEdit,
  onEdit,
}: {
  orgId: string | null
  projectId: string | null
  rules: AlertRule[]
  canEdit: boolean
  onEdit: (rule: AlertRule) => void
}) {
  const update = useUpdateAlertRule(orgId, projectId)
  const remove = useDeleteAlertRule(orgId, projectId)

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Signal</TableHead>
          <TableHead>Scope</TableHead>
          <TableHead>Severity</TableHead>
          <TableHead>Notify</TableHead>
          <TableHead>Enabled</TableHead>
          {canEdit ? <TableHead className="w-32" /> : null}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rules.map((rule) => (
          <TableRow key={rule.id}>
            <TableCell className="font-medium">{rule.name}</TableCell>
            <TableCell>
              <Badge variant="outline">{rule.signal}</Badge>
            </TableCell>
            <TableCell>{rule.agent_name || "All agents"}</TableCell>
            <TableCell>{rule.severity}</TableCell>
            <TableCell>{rule.channel_config.to ?? "—"}</TableCell>
            <TableCell>
              <Button
                variant={rule.enabled ? "outline" : "ghost"}
                size="sm"
                disabled={!canEdit || update.isPending}
                onClick={() =>
                  update.mutate({
                    ruleId: rule.id,
                    input: { enabled: !rule.enabled },
                  })
                }
              >
                {rule.enabled ? "Enabled" : "Disabled"}
              </Button>
            </TableCell>
            {canEdit ? (
              <TableCell className="space-x-2 text-right">
                <Button variant="ghost" size="sm" onClick={() => onEdit(rule)}>
                  Edit
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    if (
                      window.confirm(
                        `Delete "${rule.name}"? Its alert history is deleted too.`
                      )
                    ) {
                      remove.mutate(rule.id)
                    }
                  }}
                >
                  Delete
                </Button>
              </TableCell>
            ) : null}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
