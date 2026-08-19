export type RunStatus = "running" | "completed" | "failed"

export type TerminationReason =
  | "clean"
  | "max_steps_reached"
  | "context_limit"
  | "error"
  | "loop_detected"
  | "timeout"

export interface AgentRun {
  id: string
  timestamp: string
  project_id: string
  agent_name: string
  status: RunStatus
  termination_reason?: TerminationReason
  loop_detected: boolean
  loop_step_index?: number
  total_steps: number
  total_tokens: number
  total_cost_usd?: number
  duration_ms?: number
  input?: string
  output?: string
  finished_at?: string
}

export type StepType = "think" | "tool_call" | "tool_result" | "replan"

export interface AgentStep {
  id: string
  timestamp: string
  agent_run_id: string
  step_index: number
  step_type: StepType
  content?: string
  tool_name?: string
  tool_input?: unknown
  tool_output?: unknown
  tool_success?: boolean
  tool_latency_ms?: number
  input_fingerprint?: string
  tokens?: number
  cost_usd?: number
}

export interface LoopHit {
  fingerprint: string
  hits: number
  step_indices: number[]
  tool_name?: string
}

export interface RunBucket {
  bucket: string
  total: number
  completed: number
  failed: number
  loop: number
}

export interface RunHealthRow {
  agent_name: string
  total_runs: number
  completed_runs: number
  loop_runs: number
  completion_rate: number | null
  loop_rate: number | null
  avg_cost_usd?: number
  avg_tokens: number
  prev_total_runs?: number
}
