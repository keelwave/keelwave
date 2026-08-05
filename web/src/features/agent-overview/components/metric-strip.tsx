import { Card, CardContent } from "@/components/ui/card"
import { DeltaBadge } from "@/components/delta-badge"

export interface Metric {
  label: string
  value: string
  current?: number
  previous?: number
  goodWhen?: "up" | "down"
  hasData?: boolean
}

export function MetricStrip({ items }: { items: Metric[] }) {
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      {items.map((m) => (
        <Card key={m.label}>
          <CardContent className="flex flex-col gap-1 p-4">
            <span className="text-xs text-muted-foreground">{m.label}</span>
            <span className="font-mono text-xl font-semibold tracking-tight">
              {m.value}
            </span>
            {m.current != null && m.previous != null && m.goodWhen ? (
              <DeltaBadge
                current={m.current}
                previous={m.previous}
                goodWhen={m.goodWhen}
                hasData={m.hasData}
              />
            ) : null}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
