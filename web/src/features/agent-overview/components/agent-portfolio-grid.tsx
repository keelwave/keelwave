import { Card, CardContent } from "@/components/ui/card"
import { DeltaBadge } from "@/components/delta-badge"
import { formatCost, formatPercent, formatTokens } from "@/lib/format"
import type { RunHealthRow } from "@/features/agent-runs/types"

export function AgentPortfolioGrid({ rows }: { rows: RunHealthRow[] }) {
  const sorted = [...rows].sort((a, b) => b.total_runs - a.total_runs)
  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
      {sorted.map((r) => (
        <Card key={r.agent_name}>
          <CardContent className="flex flex-col gap-2 p-4">
            <div className="flex items-center justify-between">
              <span className="font-mono text-sm font-medium">
                {r.agent_name}
              </span>
              <DeltaBadge
                current={r.total_runs}
                previous={r.prev_total_runs ?? 0}
                goodWhen="up"
              />
            </div>
            <span className="font-mono text-2xl font-semibold">
              {formatTokens(r.total_runs)}
            </span>
            <div className="flex gap-4 text-xs text-muted-foreground">
              <span>{formatPercent(r.completion_rate)} done</span>
              <span>{formatPercent(r.loop_rate)} loop</span>
              <span>{formatCost(r.avg_cost_usd)}</span>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
