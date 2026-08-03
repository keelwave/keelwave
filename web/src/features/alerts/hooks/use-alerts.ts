import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { alertApi, alertRuleApi } from "@/features/alerts/api"
import type { AlertRuleInput, AlertRulePreviewInput } from "@/features/alerts/types"

const STALE_TIME = 1000 * 30
const REFETCH_INTERVAL = 1000 * 30

export const alertKeys = {
  all: ["alerts"] as const,
  list: (projectId: string, state?: string) =>
    ["alerts", projectId, "list", state ?? "all"] as const,
  rules: (orgId: string, projectId: string) =>
    ["alerts", orgId, projectId, "rules"] as const,
}

export function useAlerts(
  projectId: string | null,
  state?: "active" | "resolved"
) {
  return useQuery({
    queryKey: alertKeys.list(projectId ?? "", state),
    queryFn: () => alertApi.list(projectId!, state),
    enabled: !!projectId,
    staleTime: STALE_TIME,
    refetchInterval: REFETCH_INTERVAL,
  })
}

export function useAlertRules(orgId: string | null, projectId: string | null) {
  return useQuery({
    queryKey: alertKeys.rules(orgId ?? "", projectId ?? ""),
    queryFn: () => alertRuleApi.list(orgId!, projectId!),
    enabled: !!orgId && !!projectId,
    staleTime: STALE_TIME,
  })
}

function useRuleMutation<TArgs>(
  orgId: string | null,
  projectId: string | null,
  fn: (args: TArgs) => Promise<unknown>
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: alertKeys.rules(orgId ?? "", projectId ?? ""),
      })
    },
  })
}

export function useCreateAlertRule(orgId: string | null, projectId: string | null) {
  return useRuleMutation(orgId, projectId, (input: AlertRuleInput) =>
    alertRuleApi.create(orgId!, projectId!, input)
  )
}

export function useUpdateAlertRule(orgId: string | null, projectId: string | null) {
  return useRuleMutation(
    orgId,
    projectId,
    ({ ruleId, input }: { ruleId: string; input: Partial<AlertRuleInput> }) =>
      alertRuleApi.update(orgId!, projectId!, ruleId, input)
  )
}

export function useDeleteAlertRule(orgId: string | null, projectId: string | null) {
  return useRuleMutation(orgId, projectId, (ruleId: string) =>
    alertRuleApi.remove(orgId!, projectId!, ruleId)
  )
}

export function usePreviewAlertRule(
  orgId: string | null,
  projectId: string | null
) {
  return useMutation({
    mutationFn: (input: AlertRulePreviewInput) =>
      alertRuleApi.preview(orgId!, projectId!, input),
  })
}
