import { useState } from "react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
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
  const [confirming, setConfirming] = useState<string | null>(null)

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
              <TableCell>
                <div className="flex justify-end gap-2">
                  <Button variant="ghost" size="sm" onClick={() => onEdit(rule)}>
                    Edit
                  </Button>
                  <AlertDialog
                    open={confirming === rule.id}
                    onOpenChange={(next) =>
                      setConfirming(next ? rule.id : null)
                    }
                  >
                    <AlertDialogTrigger
                      render={<Button variant="ghost" size="sm" />}
                    >
                      Delete
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>
                          Delete &ldquo;{rule.name}&rdquo;?
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                          Its alert history is deleted too. This cannot be
                          undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          variant="destructive"
                          disabled={remove.isPending}
                          onClick={() => {
                            remove.mutate(rule.id)
                            setConfirming(null)
                          }}
                        >
                          Delete rule
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </div>
              </TableCell>
            ) : null}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
