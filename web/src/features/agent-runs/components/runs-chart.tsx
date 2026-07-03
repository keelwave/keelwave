import { Bar, BarChart, CartesianGrid, XAxis } from "recharts"

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart"
import type { ChartConfig } from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"
import { useRunsTimeseries } from "@/features/agent-runs/hooks/use-agent-runs"
import type { BucketSize } from "@/features/agent-runs/hooks/use-agent-runs"

const chartConfig = {
  completed: { label: "Completed", color: "#10b981" },
  failed: { label: "Failed", color: "#ef4444" },
  loop: { label: "Loop", color: "#f59e0b" },
} satisfies ChartConfig

function formatTick(iso: string, bucket: BucketSize): string {
  const d = new Date(iso)
  if (bucket === "1h") {
    return d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit" })
  }
  if (bucket === "6h") {
    return d.toLocaleString("en-US", {
      month: "short",
      day: "numeric",
      hour: "numeric",
    })
  }
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" })
}

function formatLabel(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  })
}

export function RunsChart({
  projectId,
  from,
  bucket = "1h",
}: {
  projectId: string | null
  from: string | null
  bucket?: BucketSize
}) {
  const series = useRunsTimeseries(projectId, from, bucket)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">Runs over time</CardTitle>
        <CardDescription>Outcome counts per bucket</CardDescription>
      </CardHeader>
      <CardContent>
        {series.isLoading ? (
          <Skeleton className="h-[220px] w-full" />
        ) : !series.data?.length ? (
          <div className="flex h-[220px] items-center justify-center text-sm text-muted-foreground">
            No runs in this window.
          </div>
        ) : (
          <ChartContainer config={chartConfig} className="h-[220px] w-full">
            <BarChart accessibilityLayer data={series.data}>
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey="bucket"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                tickFormatter={(v) => formatTick(v, bucket)}
              />
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    labelFormatter={(_, payload) =>
                      formatLabel(payload[0]?.payload?.bucket ?? "")
                    }
                  />
                }
              />
              <ChartLegend content={<ChartLegendContent />} />
              <Bar
                dataKey="completed"
                stackId="a"
                fill="var(--color-completed)"
              />
              <Bar dataKey="failed" stackId="a" fill="var(--color-failed)" />
              <Bar dataKey="loop" stackId="a" fill="var(--color-loop)" />
            </BarChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}
