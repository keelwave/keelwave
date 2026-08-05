import { useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  useCreateAlertRule,
  usePreviewAlertRule,
  useUpdateAlertRule,
} from "@/features/alerts/hooks/use-alerts"
import {
  comparatorOptions,
  draftClass,
  draftFromRule,
  emptyDraft,
  naturalComparator,
  toPreviewInput,
  toRuleInput,
} from "@/features/alerts/rule-form-state"
import type { RuleDraft } from "@/features/alerts/rule-form-state"
import type { AlertRule, AlertRulePreviewResult, AlertSignal } from "@/features/alerts/types"

/**
 * The UI kit has no label or switch primitive (see components/ui/), so this
 * form uses a plain styled label and a button for the one boolean it needs.
 */
function Label({
  htmlFor,
  children,
}: {
  htmlFor: string
  children: React.ReactNode
}) {
  return (
    <label htmlFor={htmlFor} className="text-sm font-medium">
      {children}
    </label>
  )
}

const SIGNALS: { value: AlertSignal; label: string; hint: string }[] = [
  { value: "loop", label: "Loop detected", hint: "An agent repeated the same tool call" },
  { value: "run_failure", label: "Run failure", hint: "Runs finishing in a failed state" },
  { value: "cost_burn", label: "Cost burn", hint: "USD spent over the window" },
  { value: "termination_shift", label: "Bad terminations", hint: "Share of error/timeout/max-steps endings" },
  { value: "tool_failure", label: "Tool failure rate", hint: "Share of failing tool calls" },
  { value: "duration_p95", label: "Duration p95", hint: "95th percentile run duration in ms" },
  { value: "eval_regression", label: "Eval regression", hint: "Average evaluation score" },
]

export function RuleForm({
  orgId,
  projectId,
  editing,
  open,
  onClose,
}: {
  orgId: string | null
  projectId: string | null
  editing: AlertRule | null
  open: boolean
  onClose: () => void
}) {
  const [draft, setDraft] = useState<RuleDraft>(emptyDraft)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [previewResult, setPreviewResult] = useState<AlertRulePreviewResult | null>(null)
  const [previewPending, setPreviewPending] = useState(false)

  const create = useCreateAlertRule(orgId, projectId)
  const update = useUpdateAlertRule(orgId, projectId)
  const preview = usePreviewAlertRule(orgId, projectId)

  useEffect(() => {
    setDraft(editing ? draftFromRule(editing) : emptyDraft())
    setPreviewResult(null)
    setPreviewPending(false)
  }, [editing, open])

  const isAggregate = draftClass(draft) === "aggregate"

  // Every preview request carries the sequence number of the draft it was built
  // from. If the draft has changed by the time a response lands, the response is
  // stale (edited-past) and gets dropped instead of rendering a verdict for
  // values no longer on screen.
  const previewSeq = useRef(0)

  // Debounced preview: only aggregate drafts have a metric to evaluate.
  useEffect(() => {
    // Bump on every draft change, not at dispatch time: an in-flight request
    // from the previous draft can land inside the debounce window and would
    // otherwise still match the sequence.
    const seq = ++previewSeq.current
    const input = toPreviewInput(draft)
    if (!input) {
      setPreviewResult(null)
      setPreviewPending(false)
      return
    }
    setPreviewPending(true)
    const timer = setTimeout(() => {
      preview.mutate(input, {
        onSuccess: (result) => {
          if (seq !== previewSeq.current) return
          setPreviewResult(result)
          setPreviewPending(false)
        },
        onError: () => {
          if (seq !== previewSeq.current) return
          setPreviewResult(null)
          setPreviewPending(false)
        },
      })
    }, 400)
    return () => clearTimeout(timer)
  }, [
    draft.signal,
    draft.comparator,
    draft.threshold,
    draft.windowSeconds,
    draft.agentName,
    draft.evaluateOverWindow,
    draft.minRequests,
  ])

  const set = <TKey extends keyof RuleDraft>(key: TKey, value: RuleDraft[TKey]) =>
    setDraft((d) => ({ ...d, [key]: value }))

  // The comparator select only ever offers the current signal's valid
  // direction (see comparatorOptions), so switching to a signal whose natural
  // direction differs from the current comparator must reset it — otherwise
  // Create submits a comparator the server rejects with a 400.
  const setSignal = (signal: AlertSignal) =>
    setDraft((d) => ({ ...d, signal, comparator: naturalComparator(signal) }))

  // run_failure flips between event and aggregate class depending on this
  // toggle; only in the aggregate case does the comparator direction matter,
  // but resetting it either way keeps the field never stale.
  const setEvaluateOverWindow = (evaluateOverWindow: boolean) =>
    setDraft((d) => ({
      ...d,
      evaluateOverWindow,
      comparator: naturalComparator(d.signal),
    }))

  const submit = () => {
    const input = toRuleInput(draft)
    if (editing) {
      update.mutate({ ruleId: editing.id, input }, { onSuccess: onClose })
    } else {
      create.mutate(input, { onSuccess: onClose })
    }
  }

  const pending = create.isPending || update.isPending

  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent className="w-full gap-5 overflow-y-auto px-4 pb-4 sm:max-w-lg">
        <SheetHeader className="px-0 pt-4 pb-0">
          <SheetTitle>{editing ? "Edit alert rule" : "New alert rule"}</SheetTitle>
          <SheetDescription>
            Pick what to watch; the rest of the form follows.
          </SheetDescription>
        </SheetHeader>

        <div className="space-y-2">
          <Label htmlFor="rule-name">Name</Label>
          <Input
            id="rule-name"
            value={draft.name}
            onChange={(e) => set("name", e.target.value)}
            placeholder="Cost burn over $5/hr"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="rule-signal">Signal</Label>
          <select
            id="rule-signal"
            className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
            value={draft.signal}
            onChange={(e) => setSignal(e.target.value as AlertSignal)}
          >
            {SIGNALS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
          <p className="text-muted-foreground text-xs">
            {SIGNALS.find((s) => s.value === draft.signal)?.hint}
          </p>
        </div>

        {draft.signal === "run_failure" ? (
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Evaluate over a window</span>
            <Button
              variant={draft.evaluateOverWindow ? "outline" : "ghost"}
              size="sm"
              onClick={() => setEvaluateOverWindow(!draft.evaluateOverWindow)}
            >
              {draft.evaluateOverWindow ? "Windowed" : "Per run"}
            </Button>
          </div>
        ) : null}

        <div className="space-y-2">
          <Label htmlFor="rule-agent">Agent (blank = all agents)</Label>
          <Input
            id="rule-agent"
            value={draft.agentName}
            onChange={(e) => set("agentName", e.target.value)}
            placeholder="research-agent"
          />
        </div>

        {isAggregate ? (
          <div className="grid grid-cols-3 gap-3">
            <div className="space-y-2">
              <Label htmlFor="rule-comparator">Comparator</Label>
              <select
                id="rule-comparator"
                className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
                value={draft.comparator}
                onChange={(e) => set("comparator", e.target.value)}
              >
                {comparatorOptions(draft.signal).map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-threshold">Threshold</Label>
              <Input
                id="rule-threshold"
                type="number"
                value={draft.threshold}
                onChange={(e) => set("threshold", Number(e.target.value))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-window">Window (s)</Label>
              <Input
                id="rule-window"
                type="number"
                min={1}
                value={draft.windowSeconds}
                onChange={(e) => set("windowSeconds", Number(e.target.value))}
              />
            </div>
          </div>
        ) : null}

        {isAggregate && previewResult ? (
          <div
            className={`bg-muted/50 rounded-md border p-3 text-sm ${
              previewPending ? "opacity-50" : ""
            }`}
          >
            {previewResult.sample_count < draft.minRequests ? (
              <span className="text-muted-foreground">
                Not enough data to evaluate ({previewResult.sample_count} samples,
                rule needs {draft.minRequests}).
              </span>
            ) : (
              <span>
                Right now: <strong>{previewResult.value.toFixed(2)}</strong> over{" "}
                {previewResult.sample_count} samples —{" "}
                {previewResult.would_breach ? (
                  <strong className="text-destructive">would breach</strong>
                ) : (
                  <>below your threshold</>
                )}
                .
              </span>
            )}
            {previewPending ? (
              <p className="text-muted-foreground mt-1 text-xs">Updating…</p>
            ) : null}
          </div>
        ) : null}

        <div className="space-y-2">
          <Label htmlFor="rule-severity">Severity</Label>
          <select
            id="rule-severity"
            className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
            value={draft.severity}
            onChange={(e) =>
              set("severity", e.target.value as RuleDraft["severity"])
            }
          >
            <option value="page">page</option>
            <option value="warn">warn</option>
            <option value="digest">digest</option>
          </select>
        </div>

        <div className="space-y-2">
          <Label htmlFor="rule-recipient">Notify (email)</Label>
          <Input
            id="rule-recipient"
            type="email"
            value={draft.recipient}
            onChange={(e) => set("recipient", e.target.value)}
            placeholder="ops@acme.com"
          />
        </div>

        <Button
          variant="ghost"
          size="sm"
          className="self-start"
          onClick={() => setShowAdvanced((v) => !v)}
        >
          {showAdvanced ? "Hide advanced" : "Advanced timing"}
        </Button>

        {showAdvanced ? (
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-2">
              <Label htmlFor="rule-for">For (s)</Label>
              <Input
                id="rule-for"
                type="number"
                value={draft.forSeconds}
                onChange={(e) => set("forSeconds", Number(e.target.value))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-keep">Keep firing (s)</Label>
              <Input
                id="rule-keep"
                type="number"
                value={draft.keepFiringForSeconds}
                onChange={(e) =>
                  set("keepFiringForSeconds", Number(e.target.value))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-cooldown">Cooldown (s)</Label>
              <Input
                id="rule-cooldown"
                type="number"
                value={draft.cooldownSeconds}
                onChange={(e) => set("cooldownSeconds", Number(e.target.value))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-min">Min samples</Label>
              <Input
                id="rule-min"
                type="number"
                value={draft.minRequests}
                onChange={(e) => set("minRequests", Number(e.target.value))}
              />
            </div>
          </div>
        ) : null}

        <div className="mt-auto flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={submit}
            disabled={pending || !draft.name.trim() || !draft.recipient.trim()}
          >
            {editing ? "Save changes" : "Create rule"}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  )
}
