import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"

export function DistributionBars({
  title = "Step type distribution",
  items,
}: {
  title?: string
  items: { label: string; count: number }[]
}) {
  const total = items.reduce((a, i) => a + i.count, 0)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {items.map((i) => {
          const pct = total ? (i.count / total) * 100 : 0
          return (
            <div key={i.label} className="flex items-center gap-3">
              <span className="w-28 shrink-0 font-mono text-xs text-muted-foreground">
                {i.label}
              </span>
              <Progress value={pct} className="h-1.5 flex-1" />
              <span className="w-12 shrink-0 text-right font-mono text-xs text-muted-foreground">
                {pct.toFixed(1)}%
              </span>
            </div>
          )
        })}
      </CardContent>
    </Card>
  )
}
