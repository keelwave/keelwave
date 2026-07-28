import { env } from "@/env"

// Same-origin by default: the keelwave binary serves this dashboard, so an
// empty base makes fetches hit relative /v1/* paths on the current origin.
// VITE_KEELWAVE_API_URL overrides this for split-origin dev.
const BASE_URL = (env.VITE_KEELWAVE_API_URL ?? "").replace(/\/$/, "")

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message)
    this.name = "ApiError"
  }
}

type QueryValue = string | number | boolean | undefined | null

async function parseResponse<T>(res: Response, path: string): Promise<T> {
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
    } | null
    if (res.status === 401 && !path.includes("/auth/")) {
      window.dispatchEvent(new CustomEvent("keelwave:unauthorized"))
    }
    throw new ApiError(res.status, body?.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  const body = (await res.json()) as { data: T }
  return body.data
}

async function get<T>(
  path: string,
  params?: Record<string, QueryValue>
): Promise<T> {
  const search = new URLSearchParams()
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null)
        search.set(key, String(value))
    }
  }
  const qs = search.toString()
  const url = `${BASE_URL}${path}${qs ? `?${qs}` : ""}`
  const res = await fetch(url, {
    headers: { Accept: "application/json" },
    credentials: "include",
  })
  return parseResponse<T>(res, path)
}

async function post<T>(path: string, data?: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: data !== undefined ? JSON.stringify(data) : undefined,
    credentials: "include",
  })
  return parseResponse<T>(res, path)
}

async function patch<T>(path: string, data?: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: data !== undefined ? JSON.stringify(data) : undefined,
    credentials: "include",
  })
  return parseResponse<T>(res, path)
}

async function put<T>(path: string, data?: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: data !== undefined ? JSON.stringify(data) : undefined,
    credentials: "include",
  })
  return parseResponse<T>(res, path)
}

async function del(path: string): Promise<void> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: "DELETE",
    credentials: "include",
  })
  return parseResponse<void>(res, path)
}

export const apiClient = {
  get,
  post,
  put,
  patch,
  delete: del,
}
