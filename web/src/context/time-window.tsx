import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { useNavigate, useSearch } from "@tanstack/react-router"

import { windowFrom } from "@/lib/format"
import type { TimeWindow } from "@/lib/format"

export type BucketSize = "1h" | "6h" | "1d"

interface TimeWindowValue {
  window: TimeWindow
  setWindow: (w: TimeWindow) => void
  from: string | null // null until the persisted window resolves; gates queries
  bucket: BucketSize
}

const TimeWindowContext = createContext<TimeWindowValue | null>(null)

const STORAGE_KEY = "kw-window"

function readStored(): TimeWindow | null {
  if (typeof globalThis.window === "undefined") return null
  const saved = globalThis.window.localStorage.getItem(STORAGE_KEY)
  return saved === "24h" || saved === "7d" || saved === "30d" ? saved : null
}

function bucketFor(w: TimeWindow): BucketSize {
  return w === "24h" ? "1h" : w === "7d" ? "6h" : "1d"
}

export function TimeWindowProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const navigate = useNavigate()
  const window = useSearch({
    strict: false,
    select: (s) => (s as { window?: TimeWindow }).window ?? "30d",
  })
  const hasUrlWindow = useSearch({
    strict: false,
    select: (s) => "window" in (s as Record<string, unknown>),
  })

  const setWindow = (w: TimeWindow) => {
    if (typeof globalThis.window !== "undefined") {
      globalThis.window.localStorage.setItem(STORAGE_KEY, w)
    }
    // tab stays before window in the query string
    navigate({
      to: ".",
      search: (prev) => {
        const { tab, window: _w, ...rest } = prev
        return tab ? { tab, ...rest, window: w } : { ...rest, window: w }
      },
    })
  }

  // Restore the last window from localStorage when the URL doesn't pin one,
  // then latch ready so queries fire once against the resolved window.
  const [ready, setReady] = useState(false)
  const firstWindowRun = useRef(true)

  useEffect(() => {
    const saved = hasUrlWindow ? null : readStored()
    if (saved && saved !== window) setWindow(saved)
    else setReady(true)
  }, [])

  useEffect(() => {
    if (firstWindowRun.current) {
      firstWindowRun.current = false
      return
    }
    setReady(true)
  }, [window])

  const value = useMemo<TimeWindowValue>(
    () => ({
      window,
      setWindow,
      from: ready ? windowFrom(window) : null,
      bucket: bucketFor(window),
    }),
    [window, ready]
  )

  return <TimeWindowContext value={value}>{children}</TimeWindowContext>
}

export function useTimeWindow(): TimeWindowValue {
  const ctx = useContext(TimeWindowContext)
  if (!ctx) {
    throw new Error("useTimeWindow must be used within TimeWindowProvider")
  }
  return ctx
}
