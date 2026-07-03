import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatCost, formatPercent, formatTokens } from "@/lib/format"
import type { RunHealthRow } from "@/features/agent-runs/types"

export function RunHealthTable({ rows }: { rows: RunHealthRow[] }) {
  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Agent</TableHead>
            <TableHead className="text-right">Runs</TableHead>
            <TableHead className="text-right">Completion</TableHead>
            <TableHead className="text-right">Loop rate</TableHead>
            <TableHead className="text-right">Avg cost</TableHead>
            <TableHead className="text-right">Avg tokens</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((r) => (
            <TableRow key={r.agent_name}>
              <TableCell className="font-mono text-sm">
                {r.agent_name}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatTokens(r.total_runs)}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatPercent(r.completion_rate)}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatPercent(r.loop_rate)}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatCost(r.avg_cost_usd)}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatTokens(Math.round(r.avg_tokens))}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
