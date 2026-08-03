import { createFileRoute } from "@tanstack/react-router"
import { useMemo, useState } from "react"

import { EmptyState } from "@/components/empty-state"
import { ErrorState } from "@/components/error-state"
import { TableSkeleton } from "@/components/table-skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { AlertsTable } from "@/features/alerts/components/alerts-table"
import { RulesTable } from "@/features/alerts/components/rules-table"
import { useAlertRules, useAlerts } from "@/features/alerts/hooks/use-alerts"
import type { AlertRule } from "@/features/alerts/types"
import { useAuth, useCurrentProject } from "@/features/auth/hooks/use-auth"
import { useMyOrgRole } from "@/features/auth/hooks/use-members"

export const Route = createFileRoute("/_auth/dashboard/alerts/")({
  component: AlertsPage,
})

function AlertsPage() {
  const { currentOrg } = useAuth()
  const { currentProjectId } = useCurrentProject()
  const alerts = useAlerts(currentProjectId)
  const rules = useAlertRules(currentOrg?.id ?? null, currentProjectId)
  const role = useMyOrgRole(currentOrg?.id ?? "")
  const canEdit = role === "admin" || role === "owner"
  const [editing, setEditing] = useState<AlertRule | null>(null)

  const ruleNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const rule of rules.data ?? []) map.set(rule.id, rule.name)
    return map
  }, [rules.data])

  const active = (alerts.data ?? []).filter((a) => a.state !== "resolved")
  const history = (alerts.data ?? []).filter((a) => a.state === "resolved")

  return (
    <div className="space-y-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold">Alerts</h1>
        <p className="text-muted-foreground text-sm">
          What is firing now, and what fired recently.
        </p>
      </div>

      <Tabs defaultValue="alerts">
        <TabsList>
          <TabsTrigger value="alerts">Alerts</TabsTrigger>
          <TabsTrigger value="rules">Rules</TabsTrigger>
        </TabsList>

        <TabsContent value="alerts" className="space-y-8">
          {alerts.isPending ? (
            <TableSkeleton />
          ) : alerts.isError ? (
            <ErrorState message="Could not load alerts." />
          ) : (
            <>
              <section className="space-y-3">
                <h2 className="text-sm font-medium">Active</h2>
                {active.length ? (
                  <AlertsTable alerts={active} ruleNames={ruleNames} />
                ) : (
                  <EmptyState
                    title="Nothing firing"
                    description="No alerts are active right now. That's good."
                  />
                )}
              </section>

              <section className="space-y-3">
                <h2 className="text-sm font-medium">History</h2>
                {history.length ? (
                  <AlertsTable alerts={history} ruleNames={ruleNames} />
                ) : (
                  <EmptyState
                    title="No history yet"
                    description="Resolved alerts will appear here."
                  />
                )}
              </section>
            </>
          )}
        </TabsContent>

        <TabsContent value="rules" className="space-y-4">
          {editing ? (
            <p className="text-muted-foreground text-sm" data-testid="editing-rule-indicator">
              Editing <span className="font-medium">{editing.name}</span> — the
              edit form lands in the next task.
            </p>
          ) : null}
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
              onEdit={setEditing}
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
        </TabsContent>
      </Tabs>
    </div>
  )
}
