export function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id
}

export function formatTokens(n: number): string {
  return n.toLocaleString("en-US")
}

export function formatCost(usd?: number): string {
  if (usd == null) return "—"
  return `$${usd.toFixed(usd < 1 ? 3 : 2)}`
}

export function formatDuration(ms?: number): string {
  if (ms == null) return "—"
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function formatPercent(rate: number | null | undefined): string {
  if (rate == null) return "—"
  return `${(rate * 100).toFixed(1)}%`
}

export type TimeWindow = "24h" | "7d" | "30d"

export const TIME_WINDOWS: { value: TimeWindow; label: string }[] = [
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
]

const WINDOW_MS: Record<TimeWindow, number> = {
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
  "30d": 30 * 24 * 60 * 60 * 1000,
}

// ISO `from` timestamp for the selected window; `to` defaults to now server-side.
export function windowFrom(w: TimeWindow): string {
  return new Date(Date.now() - WINDOW_MS[w]).toISOString()
}

// tool_input / tool_output arrive base64-encoded (Go marshals []byte that way).
// Decode to a pretty JSON string for display; fall back to the raw decode.
export function decodeToolPayload(b64?: unknown): string | null {
  if (typeof b64 !== "string" || b64 === "") return null
  let raw: string
  try {
    raw = atob(b64)
  } catch {
    return String(b64)
  }
  if (raw === "" || raw === "null") return null
  try {
    return JSON.stringify(JSON.parse(raw))
  } catch {
    return raw
  }
}
