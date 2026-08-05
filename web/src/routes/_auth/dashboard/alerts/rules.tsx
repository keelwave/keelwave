import { createFileRoute } from "@tanstack/react-router"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { EmptyState } from "@/components/empty-state"
import { ErrorState } from "@/components/error-state"
import { TableSkeleton } from "@/components/table-skeleton"
import { RuleForm } from "@/features/alerts/components/rule-form"
import { RulesTable } from "@/features/alerts/components/rules-table"
import { useAlertRules } from "@/features/alerts/hooks/use-alerts"
import type { AlertRule } from "@/features/alerts/types"
import { useAuth, useCurrentProject } from "@/features/auth/hooks/use-auth"
import { useMyOrgRole } from "@/features/auth/hooks/use-members"

export const Route = createFileRoute("/_auth/dashboard/alerts/rules")({
  component: RulesPage,
})

function RulesPage() {
  const { currentOrg } = useAuth()
  const { currentProjectId } = useCurrentProject()
  const rules = useAlertRules(currentOrg?.id ?? null, currentProjectId)
  const role = useMyOrgRole(currentOrg?.id ?? "")
  const canEdit = role === "admin" || role === "owner"
  const [editing, setEditing] = useState<AlertRule | null>(null)
  const [formOpen, setFormOpen] = useState(false)

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold tracking-tight">Alert rules</h1>
          <p className="text-sm text-muted-foreground">
            The conditions that decide when an alert fires.
          </p>
        </div>
        {canEdit ? (
          <Button
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            New rule
          </Button>
        ) : null}
      </div>

      {rules.isPending ? (
        <TableSkeleton />
      ) : rules.isError ? (
        <ErrorState message="Could not load rules." />
      ) : rules.data.length ? (
        <RulesTable
          orgId={currentOrg?.id ?? null}
          projectId={currentProjectId}
          rules={rules.data}
          canEdit={canEdit}
          onEdit={(rule) => {
            setEditing(rule)
            setFormOpen(true)
          }}
        />
      ) : (
        <EmptyState
          title="No alert rules yet"
          description={
            canEdit
              ? "Create a rule to start getting notified."
              : "Ask an org admin to create one."
          }
        />
      )}

      <RuleForm
        orgId={currentOrg?.id ?? null}
        projectId={currentProjectId}
        editing={editing}
        open={formOpen}
        onClose={() => setFormOpen(false)}
      />
    </div>
  )
}
