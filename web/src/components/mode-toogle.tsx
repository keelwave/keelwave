import { Monitor, Moon, Sun } from "lucide-react"

import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

const OPTIONS = [
  { value: "system", icon: Monitor, label: "System" },
  { value: "light", icon: Sun, label: "Light" },
  { value: "dark", icon: Moon, label: "Dark" },
] as const

export function ModeToggle({ className }: { className?: string }) {
  const { theme, setTheme } = useTheme()

  return (
    <div
      className={cn(
        "inline-flex items-center gap-0.5 rounded-full border border-border bg-muted/40 p-0.5",
        className
      )}
    >
      {OPTIONS.map(({ value, icon: Icon, label }) => {
        const active = theme === value
        return (
          <button
            key={value}
            type="button"
            onClick={() => setTheme(value)}
            title={label}
            aria-label={label}
            aria-pressed={active}
            className={cn(
              "flex size-7 items-center justify-center rounded-full transition-colors",
              active
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            <Icon className="size-3.5" />
          </button>
        )
      })}
    </div>
  )
}
