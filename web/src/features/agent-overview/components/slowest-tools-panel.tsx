import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { ToolStat } from "@/features/agent-tools/types"

export function SlowestToolsPanel({ tools }: { tools: ToolStat[] }) {
  const sorted = [...tools]
    .sort((a, b) => b.p95_latency_ms - a.p95_latency_ms)
    .slice(0, 6)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">Slowest tools</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Tool</TableHead>
              <TableHead className="text-right">p50</TableHead>
              <TableHead className="text-right">p95</TableHead>
              <TableHead className="text-right">p99</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((t) => (
              <TableRow key={t.tool_name}>
                <TableCell className="font-mono text-sm">
                  {t.tool_name}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {t.p50_latency_ms}ms
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {t.p95_latency_ms}ms
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {t.p99_latency_ms}ms
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
