import { createFileRoute } from "@tanstack/react-router"
import { useMemo } from "react"

import { EmptyState } from "@/components/empty-state"
import { ErrorState } from "@/components/error-state"
import { TableSkeleton } from "@/components/table-skeleton"
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

  // A rule lookup that never completed (failed, or disabled because the org is
  // not resolved yet) is not the same as a rule that was deleted, so the table
  // gets a neutral label rather than claiming every rule is gone.
  const unknownRuleLabel = rules.isSuccess ? "Deleted rule" : "Rule unavailable"

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold tracking-tight">Alerts</h1>
        <p className="text-sm text-muted-foreground">
          What is firing now, and what fired recently.
        </p>
      </div>

      {alerts.isPending || rules.isLoading ? (
        <TableSkeleton />
      ) : alerts.isError ? (
        <ErrorState message="Could not load alerts." />
      ) : (
        <div className="flex flex-col gap-8">
          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-semibold tracking-tight">Active</h2>
            {active.length ? (
              <AlertsTable
                alerts={active}
                ruleNames={ruleNames}
                unknownRuleLabel={unknownRuleLabel}
              />
            ) : (
              <EmptyState
                title="Nothing firing"
                description="No alerts are active right now. That's good."
              />
            )}
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-semibold tracking-tight">History</h2>
            {history.length ? (
              <AlertsTable
                alerts={history}
                ruleNames={ruleNames}
                unknownRuleLabel={unknownRuleLabel}
              />
            ) : (
              <EmptyState
                title="No history yet"
                description="Resolved alerts will appear here."
              />
            )}
          </section>
        </div>
      )}
    </div>
  )
}
