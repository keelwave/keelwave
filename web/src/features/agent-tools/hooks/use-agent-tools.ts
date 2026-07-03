import { useQuery } from "@tanstack/react-query"

import { apiClient } from "@/lib/api-client"
import type { ToolStat } from "@/features/agent-tools/types"

const STALE_TIME = 1000 * 30

export const agentToolKeys = {
  all: ["agent-tools"] as const,
  stats: (projectId: string, from: string | null) =>
    ["agent-tools", projectId, "stats", from] as const,
}

export function useToolStats(projectId: string | null, from: string | null) {
  return useQuery({
    queryKey: agentToolKeys.stats(projectId ?? "", from),
    queryFn: () =>
      apiClient.get<ToolStat[]>(`/v1/projects/${projectId}/agent/tools/stats`, {
        from: from!,
      }),
    enabled: !!projectId && from != null,
    staleTime: STALE_TIME,
  })
}
