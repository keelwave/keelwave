import { createFileRoute } from "@tanstack/react-router"

import { ErrorState } from "@/components/error-state"
import { TableSkeleton } from "@/components/table-skeleton"
import { EmptyState } from "@/components/empty-state"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MetricStrip } from "@/features/agent-overview/components/metric-strip"
import type { Metric } from "@/features/agent-overview/components/metric-strip"
import { DistributionBars } from "@/features/agent-overview/components/distribution-bars"
import { LatencyStrip } from "@/features/agent-overview/components/latency-strip"
import { SlowestToolsPanel } from "@/features/agent-overview/components/slowest-tools-panel"
import { TopFailuresPanel } from "@/features/agent-overview/components/top-failures-panel"
import { GrowingToolsPanel } from "@/features/agent-overview/components/growing-tools-panel"
import { AgentPortfolioGrid } from "@/features/agent-overview/components/agent-portfolio-grid"
import {
  useStepDistribution,
  useSummary,
  useTerminations,
} from "@/features/agent-overview/hooks/use-overview"
import type { SummaryResponse } from "@/features/agent-overview/types"
import { RunsChart } from "@/features/agent-runs/components/runs-chart"
import {
  useRunHealth,
  useToolStats,
} from "@/features/agent-runs/hooks/use-agent-runs"
import {
  formatCost,
  formatDuration,
  formatPercent,
  formatTokens,
} from "@/lib/format"
import { useTimeWindow } from "@/context/time-window"
import { useCurrentProject } from "@/features/auth/hooks/use-auth"

type OverviewTab = "performance" | "analytics"

export const Route = createFileRoute("/_auth/dashboard/")({
  validateSearch: (s: Record<string, unknown>): { tab: OverviewTab } => ({
    tab: s.tab === "analytics" ? "analytics" : "performance",
  }),
  component: OverviewPage,
})

function OverviewPage() {
  const { from, bucket } = useTimeWindow()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  const { currentProjectId, isLoadingProject, hasNoProjects } = useCurrentProject()

  const summary = useSummary(currentProjectId, from)
  const tools = useToolStats(currentProjectId, from)
  const health = useRunHealth(currentProjectId, from)
  const terminations = useTerminations(currentProjectId, from)
  const stepDist = useStepDistribution(currentProjectId, from)

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold tracking-tight">Overview</h1>
        <p className="text-sm text-muted-foreground">
          Agent fleet health at a glance.
        </p>
      </div>

      {isLoadingProject ? (
        <TableSkeleton rows={3} />
      ) : hasNoProjects ? (
        <EmptyState
          title="No project yet"
          description="Go to Settings → Projects & Keys to create your first project."
        />
      ) : summary.isError ? (
        <ErrorState message={summary.error.message} />
      ) : summary.isLoading ? (
        <TableSkeleton rows={3} />
      ) : !summary.data ? null : (
        <Tabs
          value={tab}
          onValueChange={(v) =>
            navigate({
              search: (p) => {
                const { tab: _t, ...rest } = p
                return { tab: v as OverviewTab, ...rest }
              },
            })
          }
          className="flex flex-col gap-6"
        >
          <TabsList>
            <TabsTrigger value="performance">Performance</TabsTrigger>
            <TabsTrigger value="analytics">Analytics</TabsTrigger>
          </TabsList>

          <TabsContent value="performance" className="flex flex-col gap-6">
            <MetricStrip items={performanceMetrics(summary.data)} />
            <LatencyStrip items={durationMetrics(summary.data)} />
            <RunsChart
              projectId={currentProjectId}
              from={from}
              bucket={bucket}
            />
            <div className="grid gap-4 lg:grid-cols-2">
              {tools.data ? <SlowestToolsPanel tools={tools.data} /> : null}
              {terminations.data ? (
                <TopFailuresPanel rows={terminations.data} />
              ) : null}
            </div>
            {stepDist.data ? (
              <DistributionBars
                items={stepDist.data.map((d) => ({
                  label: d.step_type,
                  count: d.count,
                }))}
              />
            ) : null}
          </TabsContent>

          <TabsContent value="analytics" className="flex flex-col gap-6">
            <MetricStrip items={analyticsMetrics(summary.data)} />
            {tools.data ? <GrowingToolsPanel tools={tools.data} /> : null}
            {health.data ? (
              <section className="flex flex-col gap-3">
                <h2 className="text-sm font-semibold tracking-tight">Agents</h2>
                <AgentPortfolioGrid rows={health.data} />
              </section>
            ) : null}
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}

function performanceMetrics(s: SummaryResponse): Metric[] {
  const c = s.current,
  p = s.previous
  const hasRuns = c.total_runs > 0
  return [
    {
      label: "Total runs",
      value: formatTokens(c.total_runs),
      current: c.total_runs,
      previous: p.total_runs,
      goodWhen: "up",
      hasData: hasRuns,
    },
    {
      label: "Completion",
      value: formatPercent(c.completion_rate),
      current: c.completion_rate,
      previous: p.completion_rate,
      goodWhen: "up",
      hasData: hasRuns,
    },
    {
      label: "Loop rate",
      value: formatPercent(c.loop_rate),
      current: c.loop_rate,
      previous: p.loop_rate,
      goodWhen: "down",
      hasData: hasRuns,
    },
    {
      label: "Avg cost",
      value: formatCost(c.avg_cost_usd),
      current: c.avg_cost_usd ?? 0,
      previous: p.avg_cost_usd ?? 0,
      goodWhen: "down",
      hasData: hasRuns && c.avg_cost_usd != null && p.avg_cost_usd != null,
    },
  ]
}

function durationMetrics(s: SummaryResponse) {
  const c = s.current
  return [
    { label: "p50", value: formatDuration(c.duration_p50_ms) },
    { label: "p95", value: formatDuration(c.duration_p95_ms) },
    { label: "p99", value: formatDuration(c.duration_p99_ms) },
  ]
}

function analyticsMetrics(s: SummaryResponse): Metric[] {
  const c = s.current,
  p = s.previous
  const stepsPerRun = c.total_runs ? c.total_steps / c.total_runs : 0
  const prevStepsPerRun = p.total_runs ? p.total_steps / p.total_runs : 0
  const hasRuns = c.total_runs > 0
  return [
    {
      label: "Unique tools",
      value: formatTokens(c.unique_tools),
      current: c.unique_tools,
      previous: p.unique_tools,
      goodWhen: "up",
      hasData: hasRuns,
    },
    {
      label: "Unique agents",
      value: formatTokens(c.unique_agents),
      current: c.unique_agents,
      previous: p.unique_agents,
      goodWhen: "up",
      hasData: hasRuns,
    },
    {
      label: "Tool calls",
      value: formatTokens(c.total_tool_calls),
      current: c.total_tool_calls,
      previous: p.total_tool_calls,
      goodWhen: "up",
      hasData: hasRuns,
    },
    {
      label: "Steps / run",
      value: stepsPerRun.toFixed(1),
      current: stepsPerRun,
      previous: prevStepsPerRun,
      goodWhen: "down",
      hasData: hasRuns,
    },
  ]
}
