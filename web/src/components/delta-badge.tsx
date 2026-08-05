import { TrendingDown, TrendingUp } from "lucide-react"

import { cn } from "@/lib/utils"

const GOOD = "text-emerald-600 dark:text-emerald-400"
const BAD = "text-red-600 dark:text-red-400"

export function DeltaBadge({
  current,
  previous,
  goodWhen,
  isNew = false,
  hasData = true,
}: {
  current: number
  previous: number
  goodWhen: "up" | "down"
  isNew?: boolean
  hasData?: boolean
}) {
  if (!hasData) {
    return <span className="font-mono text-xs text-muted-foreground">—</span>
  }
  if (!previous) {
    if (isNew) {
      return (
        <span className="font-mono text-xs text-muted-foreground">new</span>
      )
    }
    if (!current) {
      return <span className="font-mono text-xs text-muted-foreground">—</span>
    }
    const good = goodWhen === "up"
    return (
      <span
        className={cn(
          "inline-flex items-center gap-0.5 font-mono text-xs",
          good ? GOOD : BAD
        )}
        title="rose from zero"
      >
        <TrendingUp className="size-3" />
      </span>
    )
  }
  const pct = ((current - previous) / previous) * 100
  const up = pct >= 0
  const good = (up && goodWhen === "up") || (!up && goodWhen === "down")
  const Icon = up ? TrendingUp : TrendingDown

  const label =
    Math.abs(pct) > 999
      ? `${up ? ">+" : "<-"}999%`
      : `${up ? "+" : ""}${pct.toFixed(1)}%`
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 font-mono text-xs",
        good ? GOOD : BAD
      )}
    >
      <Icon className="size-3" />
      {label}
    </span>
  )
}
