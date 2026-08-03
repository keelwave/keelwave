import { createFileRoute } from "@tanstack/react-router"
import { useMemo } from "react"

import { EmptyState } from "@/components/empty-state"
import { ErrorState } from "@/components/error-state"
import { TableSkeleton } from "@/components/table-skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { AlertsTable } from "@/features/alerts/components/alerts-table"
import { useAlertRules, useAlerts } from "@/features/alerts/hooks/use-alerts"
import { useAuth, useCurrentProject } from "@/features/auth/hooks/use-auth"

export const Route = createFileRoute("/_auth/dashboard/alerts/")({
  component: AlertsPage,
})

function AlertsPage() {
  const { currentOrg } = useAuth()
  const { currentProjectId } = useCurrentProject()
  const alerts = useAlerts(currentProjectId)
  const rules = useAlertRules(currentOrg?.id ?? null, currentProjectId)

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

        <TabsContent value="rules">
          <p className="text-muted-foreground text-sm">Rules land in the next task.</p>
        </TabsContent>
      </Tabs>
    </div>
  )
}
