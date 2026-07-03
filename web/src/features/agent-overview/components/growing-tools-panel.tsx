import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { CountTrend } from "@/components/count-trend"
import { formatTokens } from "@/lib/format"
import type { ToolStat } from "@/features/agent-tools/types"

export function GrowingToolsPanel({ tools }: { tools: ToolStat[] }) {
  const sorted = [...tools]
    .sort((a, b) => b.call_count - a.call_count)
    .slice(0, 6)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">Top tools</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Tool</TableHead>
              <TableHead className="text-right">Calls</TableHead>
              <TableHead className="text-right">Trend</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((t) => (
              <TableRow key={t.tool_name}>
                <TableCell className="font-mono text-sm">
                  {t.tool_name}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {formatTokens(t.call_count)}
                </TableCell>
                <TableCell className="text-right">
                  <CountTrend
                    current={t.call_count}
                    previous={t.prev_call_count}
                    isNew={t.is_new}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
