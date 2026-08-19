import { Minus, TrendingDown, TrendingUp } from "lucide-react"

import { cn } from "@/lib/utils"

const GOOD = "text-emerald-600 dark:text-emerald-400"
const BAD = "text-red-600 dark:text-red-400"
const MUTED = "font-mono text-xs text-muted-foreground"

export type MetricKind = "count" | "rate" | "currency"

// Absolute change, not a ratio: percent change is undefined at a zero baseline
// and ambiguous on a metric that is itself a percentage.
function formatDelta(delta: number, kind: MetricKind): string {
  const sign = delta > 0 ? "+" : "−"
  const abs = Math.abs(delta)
  switch (kind) {
    case "rate":
      return `${sign}${(abs * 100).toFixed(1)}pp`
    case "currency":
      return `${sign}$${abs.toFixed(abs < 1 ? 3 : 2)}`
    default:
      return `${sign}${Number.isInteger(abs) ? abs.toLocaleString("en-US") : abs.toFixed(1)}`
  }
}

export function DeltaBadge({
  current,
  previous,
  goodWhen,
  kind = "rate",
}: {
  current: number | null | undefined
  previous: number | null | undefined
  goodWhen: "up" | "down"
  kind?: MetricKind
}) {
  if (current == null || previous == null) {
    return <span className={MUTED}>—</span>
  }

  const delta = current - previous
  if (delta === 0) {
    return (
      <span className={cn(MUTED, "inline-flex items-center gap-0.5")}>
        <Minus className="size-3" />
        {formatDelta(0, kind).replace("−", "")}
      </span>
    )
  }

  const up = delta > 0
  const good = (up && goodWhen === "up") || (!up && goodWhen === "down")
  const Icon = up ? TrendingUp : TrendingDown
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 font-mono text-xs",
        good ? GOOD : BAD
      )}
    >
      <Icon className="size-3" />
      {formatDelta(delta, kind)}
    </span>
  )
}
