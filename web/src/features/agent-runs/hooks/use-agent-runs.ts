import { useQuery } from "@tanstack/react-query"

import { apiClient } from "@/lib/api-client"
import type {
  AgentRun,
  AgentStep,
  LoopHit,
  RunBucket,
  RunHealthRow,
} from "@/features/agent-runs/types"

export { useToolStats } from "@/features/agent-tools/hooks/use-agent-tools"

export type BucketSize = "1h" | "6h" | "1d"

const STALE_TIME = 1000 * 30 // 30s — telemetry is append-heavy, keep it fresh

export interface ListRunsParams {
  from?: string | null
  to?: string
  limit?: number
  offset?: number
}

export const agentRunKeys = {
  all: ["agent-runs"] as const,
  list: (projectId: string, params?: ListRunsParams) =>
    ["agent-runs", projectId, "list", params] as const,
  detail: (projectId: string, id: string) =>
    ["agent-runs", projectId, "detail", id] as const,
  steps: (projectId: string, id: string) =>
    ["agent-runs", projectId, "steps", id] as const,
  loops: (projectId: string, id: string) =>
    ["agent-runs", projectId, "loops", id] as const,
  health: (projectId: string, from: string | null) =>
    ["agent-runs", projectId, "health", from] as const,
  timeseries: (projectId: string, from: string | null, bucket: BucketSize) =>
    ["agent-runs", projectId, "timeseries", from, bucket] as const,
}

export function useAgentRuns(
  projectId: string | null,
  params?: ListRunsParams
) {
  return useQuery({
    queryKey: agentRunKeys.list(projectId ?? "", params),
    queryFn: () =>
      apiClient.get<AgentRun[]>(`/v1/projects/${projectId}/agent/runs`, {
        ...params,
      }),
    enabled: !!projectId && params?.from != null,
    staleTime: STALE_TIME,
  })
}

export function useAgentRun(projectId: string | null, id: string) {
  return useQuery({
    queryKey: agentRunKeys.detail(projectId ?? "", id),
    queryFn: () =>
      apiClient.get<AgentRun>(`/v1/projects/${projectId}/agent/runs/${id}`),
    enabled: !!projectId && !!id,
    staleTime: STALE_TIME,
  })
}

export function useAgentSteps(projectId: string | null, id: string) {
  return useQuery({
    queryKey: agentRunKeys.steps(projectId ?? "", id),
    queryFn: () =>
      apiClient.get<AgentStep[]>(
        `/v1/projects/${projectId}/agent/runs/${id}/steps`
      ),
    enabled: !!projectId && !!id,
    staleTime: STALE_TIME,
  })
}

export function useAgentLoops(projectId: string | null, id: string) {
  return useQuery({
    queryKey: agentRunKeys.loops(projectId ?? "", id),
    queryFn: () =>
      apiClient.get<LoopHit[]>(
        `/v1/projects/${projectId}/agent/runs/${id}/loops`
      ),
    enabled: !!projectId && !!id,
    staleTime: STALE_TIME,
  })
}

export function useRunsTimeseries(
  projectId: string | null,
  from: string | null,
  bucket: BucketSize = "1h"
) {
  return useQuery({
    queryKey: agentRunKeys.timeseries(projectId ?? "", from, bucket),
    queryFn: () =>
      apiClient.get<RunBucket[]>(
        `/v1/projects/${projectId}/agent/runs/timeseries`,
        { from: from!, bucket }
      ),
    enabled: !!projectId && from != null,
    staleTime: STALE_TIME,
  })
}

export function useRunHealth(projectId: string | null, from: string | null) {
  return useQuery({
    queryKey: agentRunKeys.health(projectId ?? "", from),
    queryFn: () =>
      apiClient.get<RunHealthRow[]>(`/v1/projects/${projectId}/agent/health`, {
        from: from!,
      }),
    enabled: !!projectId && from != null,
    staleTime: STALE_TIME,
  })
}
