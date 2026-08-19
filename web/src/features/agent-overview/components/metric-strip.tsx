import { Card, CardContent } from "@/components/ui/card"
import { DeltaBadge } from "@/components/delta-badge"
import type { MetricKind } from "@/components/delta-badge"

export interface Metric {
  label: string
  value: string
  current?: number | null
  previous?: number | null
  goodWhen?: "up" | "down"
  kind?: MetricKind
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
            {m.goodWhen ? (
              <DeltaBadge
                current={m.current}
                previous={m.previous}
                goodWhen={m.goodWhen}
                kind={m.kind}
              />
            ) : null}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
