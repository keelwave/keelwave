import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatTokens } from "@/lib/format"
import type { TerminationCount } from "@/features/agent-overview/types"

export function TopFailuresPanel({ rows }: { rows: TerminationCount[] }) {
  const sorted = [...rows].sort((a, b) => b.count - a.count)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">
          Termination reasons
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Reason</TableHead>
              <TableHead className="text-right">Runs</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((r) => (
              <TableRow key={r.termination_reason}>
                <TableCell className="font-mono text-sm">
                  {r.termination_reason}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {formatTokens(r.count)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
