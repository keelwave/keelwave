export interface RunSummary {
  total_runs: number
  completed_runs: number
  loop_runs: number
  completion_rate: number | null
  loop_rate: number | null
  avg_cost_usd?: number
  avg_tokens?: number
  total_steps: number
  duration_p50_ms?: number
  duration_p95_ms?: number
  duration_p99_ms?: number
  total_tool_calls: number
  unique_tools: number
  unique_agents: number
}

export interface SummaryResponse {
  current: RunSummary
  previous: RunSummary
}

export interface StepTypeCount {
  step_type: string
  count: number
}

export interface TerminationCount {
  termination_reason: string
  count: number
}
