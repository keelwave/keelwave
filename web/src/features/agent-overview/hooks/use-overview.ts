import { useQuery } from "@tanstack/react-query"

import { apiClient } from "@/lib/api-client"
import type {
  StepTypeCount,
  SummaryResponse,
  TerminationCount,
} from "@/features/agent-overview/types"

const STALE_TIME = 1000 * 30

export const overviewKeys = {
  summary: (projectId: string, from: string | null) =>
    ["overview", projectId, "summary", from] as const,
  stepDist: (projectId: string, from: string | null) =>
    ["overview", projectId, "step-distribution", from] as const,
  terminations: (projectId: string, from: string | null) =>
    ["overview", projectId, "terminations", from] as const,
}

export function useSummary(projectId: string | null, from: string | null) {
  return useQuery({
    queryKey: overviewKeys.summary(projectId ?? "", from),
    queryFn: () =>
      apiClient.get<SummaryResponse>(
        `/v1/projects/${projectId}/agent/summary`,
        { from: from! }
      ),
    enabled: !!projectId && from != null,
    staleTime: STALE_TIME,
  })
}

export function useStepDistribution(
  projectId: string | null,
  from: string | null
) {
  return useQuery({
    queryKey: overviewKeys.stepDist(projectId ?? "", from),
    queryFn: () =>
      apiClient.get<StepTypeCount[]>(
        `/v1/projects/${projectId}/agent/steps/distribution`,
        { from: from! }
      ),
    enabled: !!projectId && from != null,
    staleTime: STALE_TIME,
  })
}

export function useTerminations(projectId: string | null, from: string | null) {
  return useQuery({
    queryKey: overviewKeys.terminations(projectId ?? "", from),
    queryFn: () =>
      apiClient.get<TerminationCount[]>(
        `/v1/projects/${projectId}/agent/runs/terminations`,
        { from: from! }
      ),
    enabled: !!projectId && from != null,
    staleTime: STALE_TIME,
  })
}
