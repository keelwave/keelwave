import { apiClient } from "@/lib/api-client"
import type {
  Alert,
  AlertRule,
  AlertRuleInput,
  AlertRulePreviewInput,
  AlertRulePreviewResult,
} from "./types"

const rulesBase = (orgId: string, projectId: string) =>
  `/v1/admin/orgs/${orgId}/projects/${projectId}/alert-rules`

export const alertApi = {
  list: (projectId: string, state?: "active" | "resolved") =>
    apiClient.get<Alert[]>(
      `/v1/projects/${projectId}/alerts/events`,
      state ? { state } : undefined
    ),
}

export const alertRuleApi = {
  list: (orgId: string, projectId: string) =>
    apiClient.get<AlertRule[]>(rulesBase(orgId, projectId)),
  create: (orgId: string, projectId: string, input: AlertRuleInput) =>
    apiClient.post<AlertRule>(rulesBase(orgId, projectId), input),
  update: (
    orgId: string,
    projectId: string,
    ruleId: string,
    input: Partial<AlertRuleInput>
  ) => apiClient.patch<AlertRule>(`${rulesBase(orgId, projectId)}/${ruleId}`, input),
  remove: (orgId: string, projectId: string, ruleId: string) =>
    apiClient.delete(`${rulesBase(orgId, projectId)}/${ruleId}`),
  preview: (orgId: string, projectId: string, input: AlertRulePreviewInput) =>
    apiClient.post<AlertRulePreviewResult>(
      `${rulesBase(orgId, projectId)}/preview`,
      input
    ),
}
